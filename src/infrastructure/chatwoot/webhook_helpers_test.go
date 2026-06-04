package chatwoot

import (
	"testing"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
)

func TestWebhookPayloadIsDeleted(t *testing.T) {
	p := WebhookPayload{ContentAttributes: ContentAttributes{Deleted: true}}
	if !p.IsDeleted() {
		t.Fatal("expected deleted=true")
	}
}

func TestShouldApplyTextEdit(t *testing.T) {
	link := &domainChatStorage.ChatwootMessageLink{
		MessageType: "text",
		LastContent: "hello",
		Status:      domainChatStorage.ChatwootLinkStatusActive,
	}

	t.Run("deleted flag skips edit", func(t *testing.T) {
		p := WebhookPayload{
			ContentAttributes: ContentAttributes{Deleted: true},
			Content:           "Ce message a été supprimé",
		}
		if ShouldApplyTextEdit(link, p) {
			t.Fatal("expected no edit when deleted")
		}
	})

	t.Run("unchanged content skips edit", func(t *testing.T) {
		p := WebhookPayload{Content: "hello"}
		if ShouldApplyTextEdit(link, p) {
			t.Fatal("expected no edit when content unchanged")
		}
	})

	t.Run("new content applies edit", func(t *testing.T) {
		p := WebhookPayload{Content: "hello world"}
		if !ShouldApplyTextEdit(link, p) {
			t.Fatal("expected edit when content changed")
		}
	})

	t.Run("delete placeholder without flag skips edit", func(t *testing.T) {
		p := WebhookPayload{Content: "Ce message a été supprimé"}
		if ShouldApplyTextEdit(link, p) {
			t.Fatal("expected no edit for delete placeholder")
		}
	})

	t.Run("media type skips edit", func(t *testing.T) {
		audioLink := &domainChatStorage.ChatwootMessageLink{
			MessageType: "audio",
			Status:      domainChatStorage.ChatwootLinkStatusActive,
		}
		p := WebhookPayload{Content: "new caption"}
		if ShouldApplyTextEdit(audioLink, p) {
			t.Fatal("expected no text edit for audio messages")
		}
	})
}

func TestEffectiveTextContent(t *testing.T) {
	p := WebhookPayload{ProcessedMessageContent: "from processed"}
	if got := EffectiveTextContent(p); got != "from processed" {
		t.Fatalf("expected processed content, got %q", got)
	}

	p = WebhookPayload{Content: "primary", ProcessedMessageContent: "secondary"}
	if got := EffectiveTextContent(p); got != "primary" {
		t.Fatalf("expected primary content, got %q", got)
	}
}
