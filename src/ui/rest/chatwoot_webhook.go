package rest

import (
	"context"
	"fmt"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	domainMessage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/message"
	domainSend "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/send"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/chatwoot"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
)

type outboundWebhookContext struct {
	deviceCtx   context.Context
	deviceID    string
	destination string
	isGroup     bool
	inboxID     int
}

func (h *ChatwootHandler) handleMessageCreated(c *fiber.Ctx, payload chatwoot.WebhookPayload) error {
	ctx, err := h.resolveOutboundWebhook(c, payload)
	if err != nil {
		return deviceUnavailableResponse(c, err)
	}
	if ctx == nil {
		return c.SendStatus(fiber.StatusOK)
	}

	if chatwoot.IsMessageSentByUs(ctx.deviceID, payload.ID) {
		logrus.Debugf("Chatwoot Webhook: Skipping echo message %d (created by our API)", payload.ID)
		return c.SendStatus(fiber.StatusOK)
	}

	var (
		whatsappMessageID string
		messageType       = "text"
		lastContent       string
	)

	if len(payload.Attachments) > 0 {
		for i, attachment := range payload.Attachments {
			msgID, msgType, err := h.handleAttachment(ctx.deviceCtx, ctx.destination, attachment, payload.Content)
			if err != nil {
				logrus.Errorf("Chatwoot Webhook: Failed to send attachment %d: %v", attachment.ID, err)
				continue
			}
			if i == 0 && msgID != "" {
				whatsappMessageID = msgID
				messageType = msgType
			}
		}
		if whatsappMessageID == "" {
			return c.SendStatus(fiber.StatusOK)
		}
	} else {
		content := chatwoot.EffectiveTextContent(payload)
		if content == "" {
			return c.SendStatus(fiber.StatusOK)
		}
		req := domainSend.MessageRequest{
			BaseRequest: domainSend.BaseRequest{Phone: ctx.destination},
			Message:     content,
		}
		resp, err := h.SendUsecase.SendText(ctx.deviceCtx, req)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"destination": ctx.destination,
				"is_group":    ctx.isGroup,
				"error":       err.Error(),
			}).Error("Chatwoot Webhook: Failed to send message (returning 200 to prevent retry)")
			return c.SendStatus(fiber.StatusOK)
		}
		whatsappMessageID = resp.MessageID
		lastContent = content
		logrus.Infof("Chatwoot Webhook: Sent text message to %s", ctx.destination)
	}

	if err := h.saveChatwootMessageLink(ctx, payload.ID, whatsappMessageID, messageType, lastContent,
		chatstorage.ChatwootLinkActionCreated, chatstorage.ChatwootLinkStatusActive); err != nil {
		logrus.WithError(err).Warnf("Chatwoot Webhook: Failed to save message link for chatwoot_id=%d", payload.ID)
	}

	return c.SendStatus(fiber.StatusOK)
}

func (h *ChatwootHandler) handleMessageUpdated(c *fiber.Ctx, payload chatwoot.WebhookPayload) error {
	ctx, err := h.resolveOutboundWebhook(c, payload)
	if err != nil {
		return deviceUnavailableResponse(c, err)
	}
	if ctx == nil {
		return c.SendStatus(fiber.StatusOK)
	}

	link, err := h.ChatStorageRepo.GetChatwootMessageLinkByChatwootID(ctx.deviceID, payload.ID)
	if err != nil {
		logrus.WithError(err).Warnf("Chatwoot Webhook: Failed to load message link for chatwoot_id=%d", payload.ID)
		return c.SendStatus(fiber.StatusOK)
	}
	if link == nil {
		logrus.Debugf("Chatwoot Webhook: No mapping for chatwoot_id=%d, skipping update", payload.ID)
		return c.SendStatus(fiber.StatusOK)
	}

	if payload.IsDeleted() {
		return h.revokeMappedMessage(c, ctx, payload, link)
	}

	if !chatwoot.ShouldApplyTextEdit(link, payload) {
		logrus.Debugf("Chatwoot Webhook: No actionable text edit for chatwoot_id=%d", payload.ID)
		return c.SendStatus(fiber.StatusOK)
	}

	return h.updateMappedMessage(c, ctx, payload, link)
}

func (h *ChatwootHandler) revokeMappedMessage(c *fiber.Ctx, ctx *outboundWebhookContext, payload chatwoot.WebhookPayload, link *chatstorage.ChatwootMessageLink) error {
	if link.Status == chatstorage.ChatwootLinkStatusRevoked {
		logrus.Debugf("Chatwoot Webhook: Message already revoked for chatwoot_id=%d", payload.ID)
		return c.SendStatus(fiber.StatusOK)
	}

	req := domainMessage.RevokeRequest{
		MessageID: link.WhatsAppMessageID,
		Phone:     ctx.destination,
	}
	_, err := h.MessageUsecase.RevokeMessage(ctx.deviceCtx, req)
	if err != nil {
		logrus.WithError(err).Errorf("Chatwoot Webhook: Failed to revoke WhatsApp message %s for chatwoot_id=%d",
			link.WhatsAppMessageID, payload.ID)
		link.Status = chatstorage.ChatwootLinkStatusFailed
		link.ActionType = chatstorage.ChatwootLinkActionDeleted
		_ = h.ChatStorageRepo.UpdateChatwootMessageLink(link)
		return c.SendStatus(fiber.StatusOK)
	}

	link.Status = chatstorage.ChatwootLinkStatusRevoked
	link.ActionType = chatstorage.ChatwootLinkActionDeleted
	if err := h.ChatStorageRepo.UpdateChatwootMessageLink(link); err != nil {
		logrus.WithError(err).Warnf("Chatwoot Webhook: Failed to update link status after revoke for chatwoot_id=%d", payload.ID)
	}
	logrus.Infof("Chatwoot Webhook: Revoked WhatsApp message %s for chatwoot_id=%d", link.WhatsAppMessageID, payload.ID)
	return c.SendStatus(fiber.StatusOK)
}

func (h *ChatwootHandler) updateMappedMessage(c *fiber.Ctx, ctx *outboundWebhookContext, payload chatwoot.WebhookPayload, link *chatstorage.ChatwootMessageLink) error {
	newContent := chatwoot.EffectiveTextContent(payload)
	req := domainMessage.UpdateMessageRequest{
		MessageID: link.WhatsAppMessageID,
		Phone:     ctx.destination,
		Message:   newContent,
	}
	_, err := h.MessageUsecase.UpdateMessage(ctx.deviceCtx, req)
	if err != nil {
		logrus.WithError(err).Errorf("Chatwoot Webhook: Failed to update WhatsApp message %s for chatwoot_id=%d",
			link.WhatsAppMessageID, payload.ID)
		link.Status = chatstorage.ChatwootLinkStatusFailed
		link.ActionType = chatstorage.ChatwootLinkActionUpdated
		_ = h.ChatStorageRepo.UpdateChatwootMessageLink(link)
		return c.SendStatus(fiber.StatusOK)
	}

	link.LastContent = newContent
	link.Status = chatstorage.ChatwootLinkStatusEdited
	link.ActionType = chatstorage.ChatwootLinkActionUpdated
	if err := h.ChatStorageRepo.UpdateChatwootMessageLink(link); err != nil {
		logrus.WithError(err).Warnf("Chatwoot Webhook: Failed to update link after edit for chatwoot_id=%d", payload.ID)
	}
	logrus.Infof("Chatwoot Webhook: Updated WhatsApp message %s for chatwoot_id=%d", link.WhatsAppMessageID, payload.ID)
	return c.SendStatus(fiber.StatusOK)
}

// resolveOutboundWebhook resolves device and destination for outbound agent webhooks.
// Returns (nil, nil) when the event should be ignored with HTTP 200.
// Returns (nil, err) when the device/inbox cannot be resolved (caller should respond 503).
func (h *ChatwootHandler) resolveOutboundWebhook(c *fiber.Ctx, payload chatwoot.WebhookPayload) (*outboundWebhookContext, error) {
	if !payload.IsOutboundAgentMessage() {
		return nil, nil
	}
	if payload.Conversation.InboxID == 0 {
		logrus.Warn("Chatwoot Webhook: inbox_id not found in webhook payload")
		return nil, nil
	}

	cwClient, err := h.Registry.GetClientByInboxID(payload.Conversation.InboxID)
	if err != nil {
		logrus.Errorf("Chatwoot Webhook: Failed to resolve device by inbox_id %d: %v", payload.Conversation.InboxID, err)
		return nil, fmt.Errorf("no device configured for inbox_id %d: %w", payload.Conversation.InboxID, err)
	}

	deviceID := cwClient.WADeviceID
	instance, resolvedID, err := h.DeviceManager.ResolveDevice(deviceID)
	if err != nil {
		logrus.Errorf("Chatwoot Webhook: Failed to resolve device %s: %v", deviceID, err)
		return nil, fmt.Errorf("device %s not available: %w", deviceID, err)
	}
	logrus.Debugf("Chatwoot Webhook: Using device %s (resolved: %s) for inbox_id %d", deviceID, resolvedID, payload.Conversation.InboxID)

	deviceCtx := whatsapp.ContextWithDevice(c.UserContext(), instance)
	c.SetUserContext(deviceCtx)

	contact := payload.Conversation.Meta.Sender
	destination := resolveChatwootDestination(contact)
	if destination == "" {
		logrus.Warnf("Chatwoot Webhook: No destination phone for contact ID %d", contact.ID)
		return nil, nil
	}

	isGroup := utils.IsGroupJID(destination)
	destination = utils.CleanPhoneForWhatsApp(destination)
	if !isGroup {
		destination = utils.ExtractPhoneFromJID(destination)
	}

	return &outboundWebhookContext{
		deviceCtx:   deviceCtx,
		deviceID:    deviceID,
		destination: destination,
		isGroup:     isGroup,
		inboxID:     payload.Conversation.InboxID,
	}, nil
}

func deviceUnavailableResponse(c *fiber.Ctx, err error) error {
	return c.Status(fiber.StatusServiceUnavailable).JSON(utils.ResponseData{
		Status:  fiber.StatusServiceUnavailable,
		Code:    "DEVICE_NOT_AVAILABLE",
		Message: err.Error(),
	})
}

func (h *ChatwootHandler) saveChatwootMessageLink(
	ctx *outboundWebhookContext,
	chatwootMessageID int,
	whatsappMessageID string,
	messageType string,
	lastContent string,
	actionType string,
	status string,
) error {
	if whatsappMessageID == "" {
		return fmt.Errorf("whatsapp_message_id is required")
	}
	return h.ChatStorageRepo.SaveChatwootMessageLink(&chatstorage.ChatwootMessageLink{
		DeviceID:          ctx.deviceID,
		InboxID:           ctx.inboxID,
		ChatwootMessageID: chatwootMessageID,
		WhatsAppMessageID: whatsappMessageID,
		ChatJID:           ctx.destination,
		MessageType:       messageType,
		LastContent:       lastContent,
		ActionType:        actionType,
		Status:            status,
	})
}
