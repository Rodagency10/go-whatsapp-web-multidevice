package chatwoot

import (
	"time"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
)

// TypingDurationOptions controls human-like typing delay calculation.
type TypingDurationOptions struct {
	BaseMs     int
	PerCharMs  int
	MinMs      int
	MaxMs      int
	MediaMs    int
	TextLength int
	HasMedia   bool
}

// DefaultTypingDurationOptions returns options from application config.
func DefaultTypingDurationOptions(payload WebhookPayload) TypingDurationOptions {
	return TypingDurationOptions{
		BaseMs:     config.ChatwootHumanTypingBaseMs,
		PerCharMs:  config.ChatwootHumanTypingPerCharMs,
		MinMs:      config.ChatwootHumanTypingMinMs,
		MaxMs:      config.ChatwootHumanTypingMaxMs,
		MediaMs:    config.ChatwootHumanTypingMediaMs,
		TextLength: len(EffectiveTextContent(payload)),
		HasMedia:   len(payload.Attachments) > 0,
	}
}

// ComputeTypingDuration returns a credible delay before sending an outbound message.
func ComputeTypingDuration(opts TypingDurationOptions) time.Duration {
	if opts.MinMs < 0 {
		opts.MinMs = 0
	}
	if opts.MaxMs > 0 && opts.MinMs > opts.MaxMs {
		opts.MinMs, opts.MaxMs = opts.MaxMs, opts.MinMs
	}

	var ms int
	if opts.TextLength == 0 && opts.HasMedia {
		ms = opts.MediaMs
	} else {
		ms = opts.BaseMs + opts.TextLength*opts.PerCharMs
	}

	if ms < opts.MinMs {
		ms = opts.MinMs
	}
	if opts.MaxMs > 0 && ms > opts.MaxMs {
		ms = opts.MaxMs
	}
	if ms < 0 {
		ms = 0
	}
	return time.Duration(ms) * time.Millisecond
}

// HumanDeliveryEnabled reports whether the full human delivery pipeline is active.
func HumanDeliveryEnabled() bool {
	return config.ChatwootHumanDeliveryEnabled
}
