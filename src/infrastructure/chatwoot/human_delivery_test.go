package chatwoot

import (
	"testing"
	"time"
)

func TestComputeTypingDuration(t *testing.T) {
	t.Run("text scales with length", func(t *testing.T) {
		got := ComputeTypingDuration(TypingDurationOptions{
			BaseMs: 1000, PerCharMs: 50, MinMs: 2500, MaxMs: 12000,
			TextLength: 10,
		})
		if got != 2500*time.Millisecond {
			t.Fatalf("expected min clamp 2500ms, got %v", got)
		}

		got = ComputeTypingDuration(TypingDurationOptions{
			BaseMs: 800, PerCharMs: 40, MinMs: 1200, MaxMs: 12000,
			TextLength: 100,
		})
		if got != 4800*time.Millisecond {
			t.Fatalf("expected 4800ms, got %v", got)
		}
	})

	t.Run("media without text uses media ms", func(t *testing.T) {
		got := ComputeTypingDuration(TypingDurationOptions{
			BaseMs: 800, PerCharMs: 40, MinMs: 1200, MaxMs: 12000, MediaMs: 2500,
			HasMedia: true,
		})
		if got != 2500*time.Millisecond {
			t.Fatalf("expected 2500ms, got %v", got)
		}
	})

	t.Run("max clamp", func(t *testing.T) {
		got := ComputeTypingDuration(TypingDurationOptions{
			BaseMs: 800, PerCharMs: 40, MinMs: 1200, MaxMs: 3000,
			TextLength: 500,
		})
		if got != 3000*time.Millisecond {
			t.Fatalf("expected max 3000ms, got %v", got)
		}
	})
}
