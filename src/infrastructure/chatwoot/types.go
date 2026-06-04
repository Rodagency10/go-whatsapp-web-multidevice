package chatwoot

type Contact struct {
	ID               int            `json:"id"`
	Name             string         `json:"name"`
	Email            string         `json:"email"`
	PhoneNumber      string         `json:"phone_number"`
	Identifier       string         `json:"identifier"`
	CustomAttributes map[string]any `json:"custom_attributes"`
}

type Conversation struct {
	ID        int    `json:"id"`
	ContactID int    `json:"contact_id"`
	InboxID   int    `json:"inbox_id"`
	Status    string `json:"status"`
}

type Message struct {
	ID          int    `json:"id"`
	Content     string `json:"content"`
	MessageType string `json:"message_type"`
	Private     bool   `json:"private"`
	ContentType string `json:"content_type"`
}

type CreateContactRequest struct {
	InboxID          int            `json:"inbox_id"`
	Name             string         `json:"name"`
	PhoneNumber      string         `json:"phone_number,omitempty"`
	Identifier       string         `json:"identifier,omitempty"`
	CustomAttributes map[string]any `json:"custom_attributes"`
}

type CreateConversationRequest struct {
	InboxID   int    `json:"inbox_id"`
	ContactID int    `json:"contact_id"`
	Status    string `json:"status"`
}

type CreateMessageRequest struct {
	Content     string `json:"content"`
	MessageType string `json:"message_type"`
	Private     bool   `json:"private"`
}

type ContentAttributes struct {
	Deleted bool `json:"deleted"`
}

type WebhookPayload struct {
	ID                      int                 `json:"id"`
	Event                   string              `json:"event"`
	MessageType             string              `json:"message_type"`
	Content                 string              `json:"content"`
	ProcessedMessageContent string              `json:"processed_message_content"`
	Private                 bool                `json:"private"`
	ContentAttributes       ContentAttributes   `json:"content_attributes"`
	Account                 Account             `json:"account"`
	Conversation            ConversationWebhook `json:"conversation"`
	Sender                  Contact             `json:"sender"`
	Attachments             []Attachment        `json:"attachments"`
}

// IsDeleted reports whether Chatwoot marked this message as deleted (via message_updated).
func (p WebhookPayload) IsDeleted() bool {
	return p.ContentAttributes.Deleted
}

// IsOutboundAgentMessage returns true for non-private outgoing agent messages.
func (p WebhookPayload) IsOutboundAgentMessage() bool {
	return p.MessageType == "outgoing" && !p.Private
}

type Attachment struct {
	ID        int    `json:"id"`
	FileType  string `json:"file_type"`
	DataURL   string `json:"data_url"`
	ThumbURL  string `json:"thumb_url"`
	Extension string `json:"extension"`
}

type ConversationWebhook struct {
	ID      int              `json:"id"`
	InboxID int              `json:"inbox_id"`
	Meta    ConversationMeta `json:"meta"`
}

type ConversationMeta struct {
	Sender Contact `json:"sender"`
}

type Account struct {
	ID int `json:"id"`
}
