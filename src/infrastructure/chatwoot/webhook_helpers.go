package chatwoot

import (
	"strings"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
)

// Placeholder texts Chatwoot uses when a message is deleted in the UI.
var deletedContentPlaceholders = []string{
	"Ce message a été supprimé",
	"This message was deleted",
}

// EffectiveTextContent returns the best available text body from a webhook payload.
func EffectiveTextContent(p WebhookPayload) string {
	if strings.TrimSpace(p.Content) != "" {
		return strings.TrimSpace(p.Content)
	}
	return strings.TrimSpace(p.ProcessedMessageContent)
}

// IsDeletedPlaceholder reports whether content is Chatwoot's delete notice, not user text.
func IsDeletedPlaceholder(content string) bool {
	trimmed := strings.TrimSpace(content)
	for _, placeholder := range deletedContentPlaceholders {
		if trimmed == placeholder {
			return true
		}
	}
	return false
}

// ShouldApplyTextEdit decides if a message_updated event should call WhatsApp BuildEdit.
func ShouldApplyTextEdit(link *domainChatStorage.ChatwootMessageLink, p WebhookPayload) bool {
	if p.IsDeleted() {
		return false
	}
	if link == nil || link.Status == domainChatStorage.ChatwootLinkStatusRevoked {
		return false
	}
	if link.MessageType != "text" {
		return false
	}
	newContent := EffectiveTextContent(p)
	if newContent == "" || IsDeletedPlaceholder(newContent) {
		return false
	}
	return newContent != link.LastContent
}

// PrimaryAttachmentType returns the file_type of the first attachment, or empty.
func PrimaryAttachmentType(p WebhookPayload) string {
	if len(p.Attachments) == 0 {
		return ""
	}
	return p.Attachments[0].FileType
}
