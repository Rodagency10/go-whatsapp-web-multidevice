# Chatwoot Integration

This document provides comprehensive documentation for integrating Go WhatsApp Web Multidevice with Chatwoot for customer support.

## Overview

The Chatwoot integration allows you to:
- Receive WhatsApp messages in your Chatwoot inbox
- Reply to WhatsApp messages directly from Chatwoot
- Support text messages, images, audio, video, and file attachments
- Handle both individual chats and group conversations
- **Multi-device support**: Configure different Chatwoot inboxes per WhatsApp device

## Prerequisites

Before setting up the integration, ensure you have:

1. **Go WhatsApp Web Multidevice** running and accessible via a public URL
2. **Chatwoot** instance (self-hosted or cloud) with admin access
3. **API Channel** inbox created in Chatwoot for each WhatsApp device
4. At least one WhatsApp device connected and logged in

## Architecture

### Multi-Device Model

Each WhatsApp device can have its own Chatwoot configuration:

```
Device "business" (628xxx@s.whatsapp.net)
  └─ Chatwoot: URL, token, account_id=1, inbox_id=1

Device "support" (629yyy@s.whatsapp.net)
  └─ Chatwoot: URL, token, account_id=1, inbox_id=2

Device "personal" (627zzz@s.whatsapp.net)
  └─ Chatwoot: URL, token, account_id=2, inbox_id=3
```

### Message Flow

1. **Incoming (WhatsApp → Chatwoot)**:
   - WhatsApp message received by connected device
   - Event handler processes the message
   - Message forwarded to Chatwoot API using the device's configured client
   - Contact/conversation created if needed
   - Message appears in the correct Chatwoot inbox

2. **Outgoing (Chatwoot → WhatsApp)**:
   - Agent replies in Chatwoot
   - Chatwoot sends webhook to `/chatwoot/webhook`
   - Handler resolves device from `inbox_id` in webhook payload
   - Message sent via WhatsApp using the resolved device
   - Delivery confirmed

## Configuration

### Via REST API

Configure Chatwoot per device using the REST API:

**Save Configuration:**
```bash
curl -X PUT "http://your-api:3000/devices/{device_id}/chatwoot" \
  -H "Content-Type: application/json" \
  -H "X-Device-Id: {device_id}" \
  -d '{
    "chatwoot_url": "https://app.chatwoot.com",
    "api_token": "your_api_token_here",
    "account_id": 1,
    "inbox_id": 1,
    "enabled": true
  }'
```

**Get Configuration:**
```bash
curl "http://your-api:3000/devices/{device_id}/chatwoot" \
  -H "X-Device-Id: {device_id}"
```

**Delete Configuration:**
```bash
curl -X DELETE "http://your-api:3000/devices/{device_id}/chatwoot" \
  -H "X-Device-Id: {device_id}"
```

### Via Web UI

1. Open the WhatsApp API web interface
2. Select or create a device
3. Scroll to the **Integrations** section
4. Click **Configure Chatwoot**
5. Fill in the required fields:
   - **Chatwoot URL**: Your Chatwoot instance URL
   - **API Token**: From Chatwoot Profile Settings → Access Token
   - **Account ID**: From Chatwoot URL `/app/accounts/{ID}/dashboard`
   - **Inbox ID**: From Chatwoot Inbox settings
6. Click **Save**

### Configuration Fields

| Field | Required | Description |
|-------|----------|-------------|
| `chatwoot_url` | Yes | Your Chatwoot instance URL (e.g., `https://app.chatwoot.com`) |
| `api_token` | Yes | API access token from Chatwoot Profile Settings |
| `account_id` | Yes | Your Chatwoot account ID (visible in URL) |
| `inbox_id` | Yes | The API inbox ID for this device |
| `enabled` | No | Toggle Chatwoot integration on/off (default: true) |

## Chatwoot Setup

### Step 1: Create an API Channel Inbox

1. Log in to your Chatwoot dashboard
2. Navigate to **Settings** > **Inboxes**
3. Click **Add Inbox**
4. Select **API** as the channel type
5. Configure the inbox:
   - **Name**: WhatsApp - Business (or any descriptive name)
   - **Webhook URL**: Leave empty for now (we'll configure this in Step 4)
6. Click **Create Inbox**
7. Note down the **Inbox ID** (visible in the URL or inbox settings)

> **Note**: Create one inbox per WhatsApp device you want to integrate.

### Step 2: Get Your API Token

1. Navigate to **Settings** > **Profile Settings**
2. Scroll to **Access Token** section
3. Copy your API access token

### Step 3: Find Your Account ID

Your account ID is visible in the URL when logged into Chatwoot:
```
https://app.chatwoot.com/app/accounts/[ACCOUNT_ID]/dashboard
```

### Step 4: Configure the Webhook

1. Navigate to **Settings** > **Integrations** > **Webhooks**
2. Click **Add new webhook**
3. Configure:
   - **URL**: `https://your-whatsapp-api.com/chatwoot/webhook`
   - **Events**: Select `message_created`
4. Click **Create**

> **Important:** The webhook URL must be publicly accessible. If you're running locally, use a tunneling service like ngrok.

## Message History Sync

The history sync feature allows you to import existing WhatsApp message history into Chatwoot.

### Via REST API

**Start Sync:**
```bash
curl -X POST "http://your-api:3000/devices/{device_id}/chatwoot/sync" \
  -H "Content-Type: application/json" \
  -H "X-Device-Id: {device_id}" \
  -d '{
    "days_limit": 7,
    "include_media": true,
    "include_groups": true
  }'
```

**Check Sync Status:**
```bash
curl "http://your-api:3000/devices/{device_id}/chatwoot/sync/status" \
  -H "X-Device-Id: {device_id}"
```

### Via Web UI

1. Go to the **Integrations** section
2. Click **Sync History** on the configured device
3. Configure sync options:
   - Days of history to sync
   - Include media attachments
   - Include group chats
4. Click **Start Sync**

### Sync Options

| Option | Default | Description |
|--------|---------|-------------|
| `days_limit` | 3 | Number of days of history to import |
| `include_media` | true | Download and sync media attachments |
| `include_groups` | true | Include group chat messages |

### Notes

- Messages are prefixed with their original timestamp for context: `[2024-01-15 14:30] Hello!`
- Group messages include the sender name: `[2024-01-15 14:30] John: Hello!`
- Media older than ~2 weeks may be unavailable on WhatsApp servers
- The sync runs in the background and can be monitored via the status endpoint
- Only one sync can run per device at a time

## Supported Features

### Incoming Messages (WhatsApp → Chatwoot)

| Message Type | Supported | Notes |
|--------------|-----------|-------|
| Text | ✅ | Full text content preserved |
| Images | ✅ | Displayed as attachments |
| Audio | ✅ | Displayed as attachments |
| Video | ✅ | Displayed as attachments |
| Documents | ✅ | Displayed as attachments |
| Stickers | ✅ | Displayed as image attachments |
| Location | ✅ | Shown as text with coordinates |
| Contacts | ✅ | vCard information preserved |

**Outgoing messages (sent from your own WhatsApp device)** are automatically forwarded to Chatwoot as `outgoing` messages.

### Outgoing Messages (Chatwoot → WhatsApp)

| Message Type | Supported | Notes |
|--------------|-----------|-------|
| Text | ✅ | - |
| Images | ✅ | Sent with optional caption |
| Audio | ✅ | Sent as voice note (PTT) |
| Video | ✅ | - |
| Files | ✅ | Any file type supported |

### Group Support

- Groups are automatically detected by JID format (`@g.us`)
- Group name is used as contact name in Chatwoot
- Replies go to the correct group chat
- Group messages include sender name prefix

## API Reference

### Chatwoot Configuration Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/devices/{device_id}/chatwoot` | GET | Get Chatwoot config for device |
| `/devices/{device_id}/chatwoot` | PUT | Create/update Chatwoot config |
| `/devices/{device_id}/chatwoot` | DELETE | Delete Chatwoot config |
| `/devices/{device_id}/chatwoot/sync` | POST | Start history sync |
| `/devices/{device_id}/chatwoot/sync/status` | GET | Get sync status |

### Webhook Endpoint

**URL:** `POST /chatwoot/webhook`

**Headers:**
- `Content-Type: application/json`

**Request Body:** Standard Chatwoot webhook payload (must include `conversation.inbox_id`)

**Response Codes:**
- `200 OK` - Message processed (or skipped)
- `503 Service Unavailable` - No device configured for the inbox_id

### Related Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/devices` | GET | List all registered devices |
| `/devices/{id}` | GET | Get device details |
| `/devices/{id}/status` | GET | Check device connection status |

## Troubleshooting

### Outbound Messages Not Sending

**Symptoms:** Messages typed in Chatwoot are not delivered to WhatsApp

**Possible Causes & Solutions:**

1. **Device not configured for inbox**
   - Check logs for "No device configured for inbox_id" errors
   - Ensure the device has a Chatwoot configuration with the correct `inbox_id`
   - Verify via API: `curl http://your-api:3000/devices/{device_id}/chatwoot`

2. **Webhook not configured**
   - Verify the webhook URL in Chatwoot settings
   - Ensure the URL is publicly accessible
   - Check that `message_created` event is selected

3. **Device not logged in**
   - Check device status: `curl http://your-api:3000/devices/{device_id}/status`
   - Reconnect the device if disconnected

### Incoming Messages Not Appearing in Chatwoot

**Symptoms:** WhatsApp messages are not showing in Chatwoot inbox

**Possible Causes & Solutions:**

1. **Chatwoot not configured for device**
   - Verify the device has a Chatwoot configuration
   - Check `enabled` is set to `true`

2. **Invalid API credentials**
   - Double-check `api_token`
   - Verify `account_id` and `inbox_id`

3. **Contact/Conversation issues**
   - Check API logs for contact creation errors
   - Verify Chatwoot inbox is properly configured

4. **Network connectivity**
   - Ensure the API server can reach Chatwoot URL
   - Check for firewall rules blocking outbound connections

### Debug Logging

Enable debug mode to see detailed Chatwoot integration logs:

```bash
./whatsapp rest --debug=true
```

Or via environment variable:
```bash
APP_DEBUG=true ./whatsapp rest
```

Look for log entries starting with:
- `Chatwoot Webhook:` - Webhook processing
- `Chatwoot:` - API operations (contact/conversation/message creation)
- `[CHATWOOT_REGISTRY]` - Client registry operations

### Common Error Messages

| Error | Meaning | Solution |
|-------|---------|----------|
| `DEVICE_NOT_AVAILABLE` | No device configured for inbox_id | Configure Chatwoot for the device with correct inbox_id |
| `No Chatwoot config found for inbox_id` | Webhook inbox_id not mapped to any device | Check inbox_id matches the configured value |
| `device not found` | Specified device ID doesn't exist | Check device ID spelling and registration |
| `failed to create contact` | Chatwoot API error | Verify API token and account permissions |
| `Invalid payload` | Malformed webhook request | Check Chatwoot webhook configuration |

### Verifying Webhook Connectivity

Test your webhook endpoint:

```bash
curl -X POST https://your-api.com/chatwoot/webhook \
  -H "Content-Type: application/json" \
  -d '{
    "event": "message_created",
    "message_type": "outgoing",
    "content": "Test message",
    "private": false,
    "conversation": {
      "id": 1,
      "inbox_id": 1,
      "meta": {
        "sender": {
          "id": 1,
          "phone_number": "+1234567890"
        }
      }
    }
  }'
```

Expected response: `200 OK` or error with details

## Best Practices

1. **Use dedicated inboxes** for each WhatsApp device
2. **Set up monitoring** for device connection status
3. **Configure auto-reconnect** to maintain service availability
4. **Test webhook connectivity** before going live
5. **Use HTTPS** for webhook URLs in production
6. **Monitor logs** for failed message deliveries
7. **Use the web UI** for easy per-device configuration

## Security Considerations

- Keep API tokens secure and rotate periodically
- Use HTTPS for all webhook communications
- Consider network-level restrictions on the webhook endpoint
- Monitor for unusual activity in Chatwoot logs
- Use strong authentication for the WhatsApp API (`APP_BASIC_AUTH`)
- **Note:** The `/chatwoot/webhook` endpoint is excluded from basic auth to allow Chatwoot to send webhooks without credentials. The configuration and sync endpoints require authentication.
