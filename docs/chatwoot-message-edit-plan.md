# Chatwoot → WhatsApp Edit/Delete Plan

This document defines the implementation plan for handling Chatwoot outbound webhook events that represent message creation, deletion, and editing, while keeping the WhatsApp side as the source of truth for the actual message action.

## Goal

When an agent acts in Chatwoot, WAPI must be able to:

1. send the outbound message to WhatsApp,
2. remember which Chatwoot message created which WhatsApp message,
3. react to message deletion events,
4. react to message edit events,
5. keep the inbox-to-device routing stable.

The key point is that message edits must not be ignored. Chatwoot already exposes a dedicated update endpoint, and WAPI already exposes a WhatsApp message update endpoint, so the plan must wire them together.

## Existing WAPI Endpoints

The relevant WAPI message endpoints already exist:

| WAPI Endpoint                 | Method | Purpose                                    |
| ----------------------------- | -----: | ------------------------------------------ |
| `/message/:message_id/update` | `POST` | Update an existing WhatsApp message        |
| `/message/:message_id/revoke` | `POST` | Revoke/delete an existing WhatsApp message |
| `/send/message`               | `POST` | Send a new text message                    |
| `/send/image`                 | `POST` | Send an image message                      |
| `/send/audio`                 | `POST` | Send an audio message                      |
| `/send/video`                 | `POST` | Send a video message                       |
| `/send/file`                  | `POST` | Send a file message                        |

The corresponding request payload for the edit endpoint is:

```json
{
  "phone": "22897986520",
  "message": "new content"
}
```

The revoke endpoint uses the WhatsApp message ID and the destination phone number.

## Chatwoot Events To Handle

Chatwoot does **not** emit `message_deleted`. Deletions arrive as `message_updated` with `content_attributes.deleted: true`.

| Chatwoot event                                        | Meaning                                        | WAPI action                               |
| ----------------------------------------------------- | ---------------------------------------------- | ----------------------------------------- |
| `message_created`                                     | A new outbound message was created in Chatwoot | Send to WhatsApp and store the ID mapping |
| `message_updated` + `content_attributes.deleted=true` | A message was deleted in Chatwoot              | Revoke the mapped WhatsApp message        |
| `message_updated` + text content changed              | A message was edited in Chatwoot (text only)   | Update the mapped WhatsApp message        |

Other `message_updated` payloads (status, upload complete, unchanged body) are ignored with HTTP 200.

## Data Model

The missing piece is a durable mapping between the Chatwoot message and the WhatsApp message.

### Required Mapping

For every successful outbound message created from Chatwoot, store:

| Field                 | Description                                                          |
| --------------------- | -------------------------------------------------------------------- |
| `device_id`           | Stable WhatsApp device identity, already standardized to JID in WAPI |
| `inbox_id`            | Chatwoot inbox ID used to route the message                          |
| `chatwoot_message_id` | Chatwoot message ID from the webhook payload                         |
| `whatsapp_message_id` | WhatsApp message ID returned by WAPI                                 |
| `chat_jid`            | Destination WhatsApp chat JID                                        |
| `message_type`        | `text`, `image`, `audio`, `video`, `file`, etc.                      |
| `action_type`         | `created`, `updated`, `deleted`                                      |
| `status`              | `active`, `edited`, `revoked`, `failed`                              |
| `payload_json`        | Raw payload snapshot for debugging                                   |

### Suggested SQLite Table

```sql
CREATE TABLE IF NOT EXISTS chatwoot_message_links (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  device_id TEXT NOT NULL,
  inbox_id INTEGER NOT NULL,
  chatwoot_message_id INTEGER NOT NULL,
  whatsapp_message_id TEXT NOT NULL,
  chat_jid TEXT NOT NULL,
  message_type TEXT NOT NULL DEFAULT 'text',
  action_type TEXT NOT NULL DEFAULT 'created',
  status TEXT NOT NULL DEFAULT 'active',
  payload_json TEXT,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  UNIQUE(device_id, chatwoot_message_id)
);
```

### Suggested Indexes

```sql
CREATE INDEX IF NOT EXISTS idx_chatwoot_message_links_device
  ON chatwoot_message_links(device_id);

CREATE INDEX IF NOT EXISTS idx_chatwoot_message_links_inbox
  ON chatwoot_message_links(inbox_id);

CREATE INDEX IF NOT EXISTS idx_chatwoot_message_links_whatsapp
  ON chatwoot_message_links(whatsapp_message_id);

CREATE INDEX IF NOT EXISTS idx_chatwoot_message_links_chatwoot
  ON chatwoot_message_links(chatwoot_message_id);

CREATE INDEX IF NOT EXISTS idx_chatwoot_message_links_status
  ON chatwoot_message_links(status);
```

## SQL Repository Contract

The chat storage interface needs a small extension for the mapping table.

### New domain record

```go
type ChatwootMessageLink struct {
    DeviceID          string
    InboxID           int
    ChatwootMessageID  int
    WhatsAppMessageID  string
    ChatJID           string
    MessageType       string
    ActionType        string
    Status            string
    PayloadJSON       string
    CreatedAt         time.Time
    UpdatedAt         time.Time
}
```

### New repository methods

```go
SaveChatwootMessageLink(link *ChatwootMessageLink) error
GetChatwootMessageLinkByChatwootID(deviceID string, chatwootMessageID int) (*ChatwootMessageLink, error)
GetChatwootMessageLinkByWhatsAppID(deviceID string, whatsappMessageID string) (*ChatwootMessageLink, error)
UpdateChatwootMessageLinkStatus(deviceID string, chatwootMessageID int, status string) error
DeleteChatwootMessageLink(deviceID string, chatwootMessageID int) error
```

### Wrapper responsibilities

The device-scoped wrapper must keep device isolation consistent:

- all lookups stay scoped to the current device,
- all writes use the current device identity,
- no cross-device fallbacks.

## Processing Flow

### 1. Outbound message created in Chatwoot

1. Chatwoot sends `message_created` to `/chatwoot/webhook`.
2. WAPI resolves the inbox via `conversation.inbox_id`.
3. WAPI resolves the WhatsApp device.
4. WAPI extracts destination JID or phone number.
5. WAPI sends the message with the appropriate send endpoint.
6. WAPI stores the resulting `whatsapp_message_id` together with the `chatwoot_message_id`.

### 2. Message deleted in Chatwoot

1. Chatwoot sends `message_updated` with `content_attributes.deleted=true`.
2. WAPI resolves the inbox and the device.
3. WAPI looks up the stored WhatsApp message ID using the Chatwoot message ID.
4. WAPI calls the revoke endpoint:

```http
POST /message/:message_id/revoke
```

5. WAPI updates the mapping row status to `revoked`.

### 3. Message edited in Chatwoot

1. Chatwoot sends `message_updated` without the delete flag.
2. WAPI resolves the inbox and the device.
3. WAPI looks up the stored WhatsApp message ID using the Chatwoot message ID.
4. WAPI calls the edit endpoint:

```http
POST /message/:message_id/update
```

5. WAPI updates the mapping row status to `edited`.

## Edit Handling Rules

Message edit handling should not be treated as optional.

### Recommended logic

- If the event says the message was deleted, always attempt revoke.
- If the event carries a new content body, always attempt update.
- If the message was not originally created by WAPI, log and stop.
- If the WhatsApp edit call fails, preserve the Chatwoot state and mark the mapping as failed for later inspection.

### Important caveat

WhatsApp edit and revoke actions only work when the original message ID is still valid for that device and chat. If the mapping is missing or stale, the operation must fail cleanly and be logged.

## Recommended API Shape In WAPI

The Chatwoot handler should branch like this:

```text
message_created
  -> send to WhatsApp
  -> store mapping

message_updated + deleted=true
  -> load mapping
  -> call revoke
  -> mark revoked

message_updated + deleted=false
  -> load mapping
  -> call update
  -> mark edited
```

## Suggested Handler Helpers

To keep `chatwoot.go` small, add helpers such as:

```go
handleChatwootMessageCreated(...)
handleChatwootMessageUpdated(...)
handleChatwootMessageDeleted(...)
resolveChatwootDestination(...)
```

## Suggested SQL Migration Order

1. Add the new `chatwoot_message_links` table.
2. Add the repository methods.
3. Wire the outbound create path to save the mapping.
4. Wire `message_updated` to use the mapping.
5. Add tests for create, delete, and edit.

## Test Cases

### Create flow

- given a valid outbound Chatwoot message,
- when WAPI sends it to WhatsApp,
- then the mapping row must be inserted with both IDs.

### Delete flow

- given a stored mapping,
- when Chatwoot sends `message_updated` with `deleted=true`,
- then WAPI must call revoke on the mapped WhatsApp message.

### Edit flow

- given a stored mapping,
- when Chatwoot sends `message_updated` with new text,
- then WAPI must call the WhatsApp update endpoint using the mapped WhatsApp message ID.

### Missing mapping

- if the Chatwoot message ID is unknown,
- WAPI must log the failure and return `200` to Chatwoot to avoid retries.

## Final Decision

This feature should be implemented as a mapping problem, not as a best-effort message replay problem.

The safe design is:

- Chatwoot remains the source of UI truth,
- WAPI remains the execution layer,
- SQLite stores the Chatwoot-to-WhatsApp correlation,
- edit and delete both use the same correlation row.
