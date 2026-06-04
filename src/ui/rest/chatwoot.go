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

type ChatwootHandler struct {
	DeviceManager   *whatsapp.DeviceManager
	ChatStorageRepo chatstorage.IChatStorageRepository
	SendUsecase     domainSend.ISendUsecase
	MessageUsecase  domainMessage.IMessageUsecase
	Registry        *chatwoot.ClientRegistry
}

func NewChatwootHandler(
	dm *whatsapp.DeviceManager,
	chatStorageRepo chatstorage.IChatStorageRepository,
	sendUsecase domainSend.ISendUsecase,
	messageUsecase domainMessage.IMessageUsecase,
	registry *chatwoot.ClientRegistry,
) *ChatwootHandler {
	return &ChatwootHandler{
		DeviceManager:   dm,
		ChatStorageRepo: chatStorageRepo,
		SendUsecase:     sendUsecase,
		MessageUsecase:  messageUsecase,
		Registry:        registry,
	}
}

// InitRestChatwoot registers Chatwoot management routes under a device-scoped router.
func InitRestChatwoot(r fiber.Router, handler *ChatwootHandler) {
	r.Get("/chatwoot", handler.GetConfig)
	r.Put("/chatwoot", handler.SaveConfig)
	r.Delete("/chatwoot", handler.DeleteConfig)
	r.Post("/chatwoot/sync", handler.SyncHistory)
	r.Get("/chatwoot/sync/status", handler.SyncStatus)
}

// GetConfig returns the Chatwoot configuration for the current device.
// GET /devices/:device_id/chatwoot
func (h *ChatwootHandler) GetConfig(c *fiber.Ctx) error {
	instance := whatsapp.InstanceFromContext(c.UserContext())
	if instance == nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
			Status:  fiber.StatusBadRequest,
			Code:    "DEVICE_REQUIRED",
			Message: "Device context is required",
		})
	}

	deviceID := instance.JID()
	config, err := h.ChatStorageRepo.GetChatwootConfig(deviceID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.ResponseData{
			Status:  fiber.StatusInternalServerError,
			Code:    "INTERNAL_ERROR",
			Message: fmt.Sprintf("Failed to get config: %v", err),
		})
	}

	if config == nil {
		return c.JSON(utils.ResponseData{
			Status:  200,
			Code:    "SUCCESS",
			Message: "No Chatwoot configuration found for this device",
			Results: map[string]any{
				"device_id":    deviceID,
				"is_configured": false,
			},
		})
	}

	// Mask the API token for security
	maskedToken := maskToken(config.APIToken)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Chatwoot configuration retrieved",
		Results: map[string]any{
			"device_id":     config.DeviceID,
			"is_configured": true,
			"chatwoot_url":  config.ChatwootURL,
			"api_token":     maskedToken,
			"account_id":    config.AccountID,
			"inbox_id":      config.InboxID,
			"enabled":       config.Enabled,
			"created_at":    config.CreatedAt,
			"updated_at":    config.UpdatedAt,
		},
	})
}

// SaveConfig creates or updates the Chatwoot configuration for the current device.
// PUT /devices/:device_id/chatwoot
func (h *ChatwootHandler) SaveConfig(c *fiber.Ctx) error {
	instance := whatsapp.InstanceFromContext(c.UserContext())
	if instance == nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
			Status:  fiber.StatusBadRequest,
			Code:    "DEVICE_REQUIRED",
			Message: "Device context is required",
		})
	}

	deviceID := instance.JID()

	var req struct {
		ChatwootURL string `json:"chatwoot_url"`
		APIToken    string `json:"api_token"`
		AccountID   int    `json:"account_id"`
		InboxID     int    `json:"inbox_id"`
		Enabled     *bool  `json:"enabled"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
			Status:  fiber.StatusBadRequest,
			Code:    "BAD_REQUEST",
			Message: "Invalid request body",
		})
	}

	if req.ChatwootURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
			Status:  fiber.StatusBadRequest,
			Code:    "VALIDATION_ERROR",
			Message: "chatwoot_url is required",
		})
	}
	if req.APIToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
			Status:  fiber.StatusBadRequest,
			Code:    "VALIDATION_ERROR",
			Message: "api_token is required",
		})
	}
	if req.AccountID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
			Status:  fiber.StatusBadRequest,
			Code:    "VALIDATION_ERROR",
			Message: "account_id must be positive",
		})
	}
	if req.InboxID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
			Status:  fiber.StatusBadRequest,
			Code:    "VALIDATION_ERROR",
			Message: "inbox_id must be positive",
		})
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	config := &chatstorage.ChatwootConfig{
		DeviceID:    deviceID,
		ChatwootURL: req.ChatwootURL,
		APIToken:    req.APIToken,
		AccountID:   req.AccountID,
		InboxID:     req.InboxID,
		Enabled:     enabled,
	}

	if err := h.ChatStorageRepo.SaveChatwootConfig(config); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.ResponseData{
			Status:  fiber.StatusInternalServerError,
			Code:    "INTERNAL_ERROR",
			Message: fmt.Sprintf("Failed to save config: %v", err),
		})
	}

	// Register or re-register the client in the registry
	if enabled {
		logrus.Infof("Chatwoot: Registering client for device %s", deviceID)
		if err := h.Registry.RegisterClient(c.UserContext(), deviceID); err != nil {
			logrus.WithError(err).Warnf("Failed to register Chatwoot client for device %s", deviceID)
		} else {
			logrus.Infof("Chatwoot: Successfully registered client for device %s", deviceID)
		}
	} else {
		logrus.Infof("Chatwoot: Removing client for device %s", deviceID)
		h.Registry.RemoveClient(deviceID)
	}

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Chatwoot configuration saved",
		Results: map[string]any{
			"device_id":  deviceID,
			"enabled":    enabled,
			"account_id": req.AccountID,
			"inbox_id":   req.InboxID,
		},
	})
}

// DeleteConfig removes the Chatwoot configuration for the current device.
// DELETE /devices/:device_id/chatwoot
func (h *ChatwootHandler) DeleteConfig(c *fiber.Ctx) error {
	instance := whatsapp.InstanceFromContext(c.UserContext())
	if instance == nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
			Status:  fiber.StatusBadRequest,
			Code:    "DEVICE_REQUIRED",
			Message: "Device context is required",
		})
	}

	deviceID := instance.JID()

	if err := h.ChatStorageRepo.DeleteChatwootConfig(deviceID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(utils.ResponseData{
			Status:  fiber.StatusInternalServerError,
			Code:    "INTERNAL_ERROR",
			Message: fmt.Sprintf("Failed to delete config: %v", err),
		})
	}

	h.Registry.RemoveClient(deviceID)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Chatwoot configuration deleted",
		Results: map[string]any{
			"device_id": deviceID,
		},
	})
}

// SyncHistory triggers a message history sync to Chatwoot for the current device.
// POST /devices/:device_id/chatwoot/sync
func (h *ChatwootHandler) SyncHistory(c *fiber.Ctx) error {
	instance := whatsapp.InstanceFromContext(c.UserContext())
	if instance == nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
			Status:  fiber.StatusBadRequest,
			Code:    "DEVICE_REQUIRED",
			Message: "Device context is required",
		})
	}

	deviceID := instance.JID()

	var req chatwoot.SyncRequest
	if err := c.BodyParser(&req); err != nil {
		req.DaysLimit = c.QueryInt("days", 3)
		req.IncludeMedia = c.QueryBool("media", true)
		req.IncludeGroups = c.QueryBool("groups", true)
	}
	if req.DaysLimit <= 0 {
		req.DaysLimit = 3
	}

	cwClient, err := h.Registry.GetClient(deviceID)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
			Status:  fiber.StatusBadRequest,
			Code:    "CHATWOOT_NOT_CONFIGURED",
			Message: "Chatwoot is not configured for this device. Configure it first via PUT /devices/:device_id/chatwoot",
		})
	}

	syncService := chatwoot.NewSyncService(cwClient, h.ChatStorageRepo)
	waClient := instance.GetClient()

	if syncService.IsRunning(deviceID) {
		progress := syncService.GetProgress(deviceID)
		return c.Status(fiber.StatusConflict).JSON(utils.ResponseData{
			Status:  fiber.StatusConflict,
			Code:    "SYNC_ALREADY_RUNNING",
			Message: "A sync is already in progress for this device",
			Results: map[string]any{
				"progress": progress,
			},
		})
	}

	opts := chatwoot.DefaultSyncOptions()
	opts.DaysLimit = req.DaysLimit
	opts.IncludeMedia = req.IncludeMedia
	opts.IncludeGroups = req.IncludeGroups

	go func() {
		ctx := context.Background()
		progress, err := syncService.SyncHistory(ctx, deviceID, waClient, opts)
		if err != nil {
			logrus.Errorf("Chatwoot Sync: Failed for device %s: %v", deviceID, err)
		} else {
			logrus.Infof("Chatwoot Sync: Completed for device %s - %d/%d messages synced",
				deviceID, progress.SyncedMessages, progress.TotalMessages)
		}
	}()

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SYNC_STARTED",
		Message: "History sync initiated in background",
		Results: map[string]any{
			"device_id":      deviceID,
			"days_limit":     opts.DaysLimit,
			"include_media":  opts.IncludeMedia,
			"include_groups": opts.IncludeGroups,
		},
	})
}

// SyncStatus returns the current sync progress for the current device.
// GET /devices/:device_id/chatwoot/sync/status
func (h *ChatwootHandler) SyncStatus(c *fiber.Ctx) error {
	instance := whatsapp.InstanceFromContext(c.UserContext())
	if instance == nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
			Status:  fiber.StatusBadRequest,
			Code:    "DEVICE_REQUIRED",
			Message: "Device context is required",
		})
	}

	deviceID := instance.JID()

	cwClient, err := h.Registry.GetClient(deviceID)
	if err != nil {
		return c.JSON(utils.ResponseData{
			Status:  200,
			Code:    "SUCCESS",
			Message: "Chatwoot not configured for this device",
			Results: map[string]any{
				"device_id": deviceID,
				"status":    "not_configured",
			},
		})
	}

	syncService := chatwoot.NewSyncService(cwClient, h.ChatStorageRepo)
	progress := syncService.GetProgress(deviceID)
	if progress == nil {
		return c.JSON(utils.ResponseData{
			Status:  200,
			Code:    "SUCCESS",
			Message: "No sync has been initiated for this device",
			Results: map[string]any{
				"device_id": deviceID,
				"status":    "idle",
			},
		})
	}

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Sync status retrieved",
		Results: progress,
	})
}

// HandleWebhook processes incoming webhooks from Chatwoot.
// POST /chatwoot/webhook (public, no auth)
func (h *ChatwootHandler) HandleWebhook(c *fiber.Ctx) error {
	logrus.Debugf("Chatwoot Webhook raw body: %s", string(c.Body()))

	var payload chatwoot.WebhookPayload
	if err := c.BodyParser(&payload); err != nil {
		return utils.ResponseError(c, "Invalid payload")
	}

	logrus.Debugf("Chatwoot Webhook: event=%s message_type=%s contact_id=%d inbox_id=%d deleted=%v",
		payload.Event, payload.MessageType, payload.Conversation.Meta.Sender.ID, payload.Conversation.InboxID, payload.IsDeleted())

	switch payload.Event {
	case "conversation_typing_on", "conversation_typing_off":
		return h.handleTypingPresence(c, payload)
	case "message_created":
		return h.handleMessageCreated(c, payload)
	case "message_updated":
		return h.handleMessageUpdated(c, payload)
	default:
		return c.SendStatus(fiber.StatusOK)
	}
}

func (h *ChatwootHandler) handleTypingPresence(c *fiber.Ctx, payload chatwoot.WebhookPayload) error {
	if payload.Conversation.InboxID == 0 {
		logrus.Warn("Chatwoot Webhook: inbox_id not found in typing webhook payload")
		return c.SendStatus(fiber.StatusOK)
	}

	cwClient, err := h.Registry.GetClientByInboxID(payload.Conversation.InboxID)
	if err != nil {
		logrus.Errorf("Chatwoot Webhook: Failed to resolve device by inbox_id %d: %v", payload.Conversation.InboxID, err)
		return c.Status(fiber.StatusServiceUnavailable).JSON(utils.ResponseData{
			Status:  fiber.StatusServiceUnavailable,
			Code:    "DEVICE_NOT_AVAILABLE",
			Message: fmt.Sprintf("No device configured for inbox_id %d", payload.Conversation.InboxID),
		})
	}

	instance, resolvedID, err := h.DeviceManager.ResolveDevice(cwClient.WADeviceID)
	if err != nil {
		logrus.Errorf("Chatwoot Webhook: Failed to resolve device %s: %v", cwClient.WADeviceID, err)
		return c.Status(fiber.StatusServiceUnavailable).JSON(utils.ResponseData{
			Status:  fiber.StatusServiceUnavailable,
			Code:    "DEVICE_NOT_AVAILABLE",
			Message: fmt.Sprintf("Device %s not available: %v", cwClient.WADeviceID, err),
		})
	}

	logrus.Debugf("Chatwoot Webhook: Using device %s (resolved: %s) for typing event inbox_id %d", cwClient.WADeviceID, resolvedID, payload.Conversation.InboxID)

	deviceCtx := whatsapp.ContextWithDevice(c.UserContext(), instance)
	c.SetUserContext(deviceCtx)

	destination := resolveChatwootDestination(payload.Conversation.Meta.Sender)
	if destination == "" {
		logrus.Warnf("Chatwoot Webhook: No destination phone for typing event contact ID %d", payload.Conversation.Meta.Sender.ID)
		return c.SendStatus(fiber.StatusOK)
	}

	action := "start"
	if payload.Event == "conversation_typing_off" {
		action = "stop"
	}

	req := domainSend.ChatPresenceRequest{
		BaseRequest: domainSend.BaseRequest{Phone: destination},
		Action:      action,
	}

	if _, err := h.SendUsecase.SendChatPresence(deviceCtx, req); err != nil {
		logrus.WithFields(logrus.Fields{
			"destination": destination,
			"action":      action,
			"error":       err.Error(),
		}).Error("Chatwoot Webhook: Failed to send typing presence (returning 200 to prevent retry)")
		return c.SendStatus(fiber.StatusOK)
	}

	logrus.Infof("Chatwoot Webhook: Sent typing presence %s to %s", action, destination)
	return c.SendStatus(fiber.StatusOK)
}

func resolveChatwootDestination(contact chatwoot.Contact) string {
	customAttrs := contact.CustomAttributes
	if val, ok := customAttrs["waha_whatsapp_jid"]; ok {
		if strVal, ok := val.(string); ok && strVal != "" {
			return strVal
		}
	}

	if contact.PhoneNumber != "" {
		return contact.PhoneNumber
	}

	return ""
}

func (h *ChatwootHandler) handleAttachment(ctx context.Context, phone string, att chatwoot.Attachment, caption string) (messageID string, messageType string, err error) {
	switch att.FileType {
	case "image":
		messageType = "image"
		req := domainSend.ImageRequest{
			BaseRequest: domainSend.BaseRequest{Phone: phone},
			Caption:     caption,
			ImageURL:    &att.DataURL,
		}
		resp, err := h.SendUsecase.SendImage(ctx, req)
		if err == nil {
			logrus.Infof("Chatwoot Webhook: Sent image attachment to %s", phone)
			return resp.MessageID, messageType, nil
		}
		return "", messageType, err

	case "audio":
		messageType = "audio"
		req := domainSend.AudioRequest{
			BaseRequest: domainSend.BaseRequest{Phone: phone},
			AudioURL:    &att.DataURL,
			PTT:         true,
		}
		resp, err := h.SendUsecase.SendAudio(ctx, req)
		if err == nil {
			logrus.Infof("Chatwoot Webhook: Sent audio attachment to %s", phone)
			return resp.MessageID, messageType, nil
		}

		logrus.Warnf("Chatwoot Webhook: Failed to send as audio (%v), retrying as file...", err)
		messageType = "file"
		reqFile := domainSend.FileRequest{
			BaseRequest: domainSend.BaseRequest{Phone: phone},
			FileURL:     &att.DataURL,
			Caption:     caption,
		}
		resp, err = h.SendUsecase.SendFile(ctx, reqFile)
		if err == nil {
			logrus.Infof("Chatwoot Webhook: Sent audio as file attachment to %s", phone)
		}
		if err != nil {
			return "", messageType, err
		}
		return resp.MessageID, messageType, nil

	case "video":
		messageType = "video"
		req := domainSend.VideoRequest{
			BaseRequest: domainSend.BaseRequest{Phone: phone},
			Caption:     caption,
			VideoURL:    &att.DataURL,
		}
		resp, err := h.SendUsecase.SendVideo(ctx, req)
		if err == nil {
			logrus.Infof("Chatwoot Webhook: Sent video attachment to %s", phone)
			return resp.MessageID, messageType, nil
		}
		return "", messageType, err

	default:
		messageType = "file"
		req := domainSend.FileRequest{
			BaseRequest: domainSend.BaseRequest{Phone: phone},
			FileURL:     &att.DataURL,
			Caption:     caption,
		}
		resp, err := h.SendUsecase.SendFile(ctx, req)
		if err == nil {
			logrus.Infof("Chatwoot Webhook: Sent file attachment to %s", phone)
			return resp.MessageID, messageType, nil
		}
		return "", messageType, err
	}
}

func maskToken(token string) string {
	if len(token) <= 8 {
		return "****"
	}
	return token[:4] + "****" + token[len(token)-4:]
}
