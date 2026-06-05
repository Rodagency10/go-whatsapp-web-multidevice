package chatwoot

import "testing"

func TestAttachmentMIMEType(t *testing.T) {
	tests := []struct {
		ext      string
		expected string
	}{
		{".ogg", "audio/ogg"},
		{".Ogg", "audio/ogg"},
		{".oga", "audio/ogg"},
		{".opus", "audio/ogg"},
		{".mp3", "audio/mpeg"},
		{".m4a", "audio/mp4"},
		{".wav", "audio/wav"},
		{".jpg", "image/jpeg"},
		{".unknown", "application/octet-stream"},
	}

	for _, tt := range tests {
		if got := attachmentMIMEType(tt.ext); got != tt.expected {
			t.Fatalf("attachmentMIMEType(%q) = %q, want %q", tt.ext, got, tt.expected)
		}
	}
}
