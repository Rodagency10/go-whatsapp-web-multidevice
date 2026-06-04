package rest

import (
	"context"
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	domainSend "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/send"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/chatwoot"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/sirupsen/logrus"
)

type messageCreatedDelivery struct {
	whatsappMessageID string
	messageType       string
	lastContent       string
}

func (h *ChatwootHandler) asyncDeliverMessageCreated(webhookCtx outboundWebhookContext, payload chatwoot.WebhookPayload) {
	instance, _, err := h.DeviceManager.ResolveDevice(webhookCtx.deviceID)
	if err != nil {
		logrus.WithError(err).Errorf("Chatwoot Human Delivery: device %s unavailable for chatwoot_id=%d",
			webhookCtx.deviceID, payload.ID)
		return
	}

	webhookCtx.deviceCtx = whatsapp.ContextWithDevice(context.Background(), instance)

	h.runHumanPreDelivery(webhookCtx.deviceCtx, &webhookCtx, payload)
	defer h.runHumanPostDelivery(webhookCtx.deviceCtx, &webhookCtx)

	result, err := h.deliverMessageCreated(webhookCtx.deviceCtx, &webhookCtx, payload)
	if err != nil {
		logrus.WithError(err).WithFields(logrus.Fields{
			"destination": webhookCtx.destination,
			"chatwoot_id": payload.ID,
		}).Error("Chatwoot Human Delivery: failed to send message")
		return
	}
	if result == nil || result.whatsappMessageID == "" {
		return
	}

	if err := h.saveChatwootMessageLink(&webhookCtx, payload.ID, result.whatsappMessageID, result.messageType, result.lastContent,
		chatstorage.ChatwootLinkActionCreated, chatstorage.ChatwootLinkStatusActive); err != nil {
		logrus.WithError(err).Warnf("Chatwoot Human Delivery: failed to save message link for chatwoot_id=%d", payload.ID)
	}
}

func (h *ChatwootHandler) runHumanPreDelivery(ctx context.Context, webhookCtx *outboundWebhookContext, payload chatwoot.WebhookPayload) {
	if config.ChatwootHumanPresenceAvailable {
		_, err := h.SendUsecase.SendPresence(ctx, domainSend.PresenceRequest{Type: "available"})
		if err != nil {
			logrus.WithError(err).Warn("Chatwoot Human Delivery: failed to send available presence")
		} else {
			logrus.Infof("Chatwoot Human Delivery: online (available) before sending to %s", webhookCtx.destination)
		}
	}

	warmRecipientPresence(ctx, webhookCtx.destination)

	if settle := chatwoot.PresenceSettleDuration(); settle > 0 {
		time.Sleep(settle)
	}

	if !config.ChatwootHumanTypingEnabled {
		return
	}

	typingDuration := chatwoot.ComputeTypingDuration(chatwoot.DefaultTypingDurationOptions(payload))
	if !h.sendTypingStart(ctx, webhookCtx.destination) {
		logrus.Warnf("Chatwoot Human Delivery: typing indicator not started for %s; sending without delay", webhookCtx.destination)
		return
	}

	logrus.Infof("Chatwoot Human Delivery: typing on %s for %s (chatwoot_id=%d)",
		webhookCtx.destination, typingDuration, payload.ID)
	h.sleepWithTypingRefresh(ctx, webhookCtx.destination, typingDuration)
}

func (h *ChatwootHandler) runHumanPostDelivery(ctx context.Context, webhookCtx *outboundWebhookContext) {
	if config.ChatwootHumanTypingEnabled {
		h.sendTypingStop(ctx, webhookCtx.destination)
	}

	if config.ChatwootHumanPresenceRestore {
		_, err := h.SendUsecase.SendPresence(ctx, domainSend.PresenceRequest{Type: "unavailable"})
		if err != nil {
			logrus.WithError(err).Warn("Chatwoot Human Delivery: failed to restore unavailable presence")
		} else {
			logrus.Debugf("Chatwoot Human Delivery: restored unavailable presence after send to %s", webhookCtx.destination)
		}
	}
}

func (h *ChatwootHandler) sendTypingStart(ctx context.Context, destination string) bool {
	_, err := h.SendUsecase.SendChatPresence(ctx, domainSend.ChatPresenceRequest{
		BaseRequest: domainSend.BaseRequest{Phone: destination},
		Action:      "start",
	})
	if err != nil {
		logrus.WithError(err).Warnf("Chatwoot Human Delivery: failed to start typing for %s", destination)
		return false
	}
	return true
}

func (h *ChatwootHandler) sendTypingStop(ctx context.Context, destination string) {
	_, err := h.SendUsecase.SendChatPresence(ctx, domainSend.ChatPresenceRequest{
		BaseRequest: domainSend.BaseRequest{Phone: destination},
		Action:      "stop",
	})
	if err != nil {
		logrus.WithError(err).Warnf("Chatwoot Human Delivery: failed to stop typing for %s", destination)
	}
}

// sleepWithTypingRefresh waits while re-sending composing so WhatsApp keeps showing typing.
func (h *ChatwootHandler) sleepWithTypingRefresh(ctx context.Context, destination string, total time.Duration) {
	if total <= 0 {
		return
	}

	refresh := chatwoot.TypingRefreshInterval()
	remaining := total
	for remaining > 0 {
		chunk := remaining
		if chunk > refresh {
			chunk = refresh
		}
		time.Sleep(chunk)
		remaining -= chunk
		if remaining <= 0 {
			break
		}
		h.sendTypingStart(ctx, destination)
	}
}

// warmRecipientPresence subscribes to the chat (best effort) so composing state is accepted by WhatsApp.
func warmRecipientPresence(ctx context.Context, destination string) {
	client := whatsapp.ClientFromContext(ctx)
	if client == nil {
		return
	}

	jid, err := utils.ValidateJidWithLogin(client, destination)
	if err != nil {
		logrus.WithError(err).Debugf("Chatwoot Human Delivery: skip presence warm-up for %s", destination)
		return
	}

	warmCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := client.SubscribePresence(warmCtx, jid); err != nil {
		logrus.WithError(err).Debugf("Chatwoot Human Delivery: SubscribePresence for %s failed (continuing)", jid.String())
		return
	}
	logrus.Debugf("Chatwoot Human Delivery: subscribed to presence for %s", jid.String())
}

func (h *ChatwootHandler) deliverMessageCreated(
	ctx context.Context,
	webhookCtx *outboundWebhookContext,
	payload chatwoot.WebhookPayload,
) (*messageCreatedDelivery, error) {
	if len(payload.Attachments) > 0 {
		var result *messageCreatedDelivery
		for i, attachment := range payload.Attachments {
			msgID, msgType, err := h.handleAttachment(ctx, webhookCtx.destination, attachment, payload.Content)
			if err != nil {
				logrus.Errorf("Chatwoot Webhook: Failed to send attachment %d: %v", attachment.ID, err)
				continue
			}
			if i == 0 && msgID != "" {
				result = &messageCreatedDelivery{
					whatsappMessageID: msgID,
					messageType:       msgType,
					lastContent:       payload.Content,
				}
			}
		}
		if result == nil {
			return nil, nil
		}
		logrus.Infof("Chatwoot Webhook: Sent attachment message to %s", webhookCtx.destination)
		return result, nil
	}

	content := chatwoot.EffectiveTextContent(payload)
	if content == "" {
		return nil, nil
	}

	req := domainSend.MessageRequest{
		BaseRequest: domainSend.BaseRequest{Phone: webhookCtx.destination},
		Message:     content,
	}
	resp, err := h.SendUsecase.SendText(ctx, req)
	if err != nil {
		return nil, err
	}

	logrus.Infof("Chatwoot Webhook: Sent text message to %s", webhookCtx.destination)
	return &messageCreatedDelivery{
		whatsappMessageID: resp.MessageID,
		messageType:       "text",
		lastContent:       content,
	}, nil
}
