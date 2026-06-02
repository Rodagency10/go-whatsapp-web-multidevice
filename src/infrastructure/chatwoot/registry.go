package chatwoot

import (
	"context"
	"fmt"
	"sync"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"github.com/sirupsen/logrus"
)

// ClientRegistry manages per-device Chatwoot client instances.
type ClientRegistry struct {
	mu       sync.RWMutex
	clients  map[string]*Client // key = device_id
	byInbox  map[int]string     // inbox_id → device_id
	repo     domainChatStorage.IChatStorageRepository
	initOnce sync.Once
}

// NewClientRegistry creates a new registry backed by the given storage repository.
func NewClientRegistry(repo domainChatStorage.IChatStorageRepository) *ClientRegistry {
	return &ClientRegistry{
		clients: make(map[string]*Client),
		byInbox: make(map[int]string),
		repo:    repo,
	}
}

// GetClient returns the Chatwoot client for a specific device.
func (r *ClientRegistry) GetClient(deviceID string) (*Client, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, ok := r.clients[deviceID]
	if !ok {
		return nil, fmt.Errorf("no Chatwoot client registered for device %s", deviceID)
	}
	return client, nil
}

// GetClientByInboxID returns the Chatwoot client associated with a given inbox ID.
func (r *ClientRegistry) GetClientByInboxID(inboxID int) (*Client, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	deviceID, ok := r.byInbox[inboxID]
	if !ok {
		return nil, fmt.Errorf("no Chatwoot config found for inbox_id %d", inboxID)
	}

	client, ok := r.clients[deviceID]
	if !ok {
		return nil, fmt.Errorf("client not initialized for device %s (inbox_id %d)", deviceID, inboxID)
	}
	return client, nil
}

// RegisterClient creates and registers a Chatwoot client for the given device.
func (r *ClientRegistry) RegisterClient(ctx context.Context, deviceID string) error {
	if r.repo == nil {
		return fmt.Errorf("storage repository is nil")
	}

	config, err := r.repo.GetChatwootConfig(deviceID)
	if err != nil {
		return fmt.Errorf("failed to load Chatwoot config for device %s: %w", deviceID, err)
	}
	if config == nil {
		return fmt.Errorf("no Chatwoot config found for device %s", deviceID)
	}
	if !config.Enabled {
		return fmt.Errorf("Chatwoot is disabled for device %s", deviceID)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove old inbox mapping if device was already registered
	if existing, ok := r.clients[deviceID]; ok {
		delete(r.byInbox, existing.InboxID)
	}

	client := NewClientFromConfig(config)
	r.clients[deviceID] = client
	r.byInbox[config.InboxID] = deviceID

	logrus.WithContext(ctx).Infof("[CHATWOOT_REGISTRY] registered client for device %s (inbox_id=%d, account_id=%d)",
		deviceID, config.InboxID, config.AccountID)
	return nil
}

// RemoveClient unregisters a Chatwoot client for a device.
func (r *ClientRegistry) RemoveClient(deviceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if client, ok := r.clients[deviceID]; ok {
		delete(r.byInbox, client.InboxID)
	}
	delete(r.clients, deviceID)

	logrus.Infof("[CHATWOOT_REGISTRY] removed client for device %s", deviceID)
}

// LoadAllConfigs loads all enabled Chatwoot configurations and initializes clients.
func (r *ClientRegistry) LoadAllConfigs(ctx context.Context) error {
	if r.repo == nil {
		return fmt.Errorf("storage repository is nil")
	}

	var loaded int
	var skipped int

	r.initOnce.Do(func() {
		configs, err := r.repo.ListChatwootConfigs()
		if err != nil {
			logrus.WithError(err).Warn("[CHATWOOT_REGISTRY] failed to list Chatwoot configs")
			return
		}

		r.mu.Lock()
		defer r.mu.Unlock()

		for _, cfg := range configs {
			if cfg == nil || !cfg.Enabled {
				skipped++
				continue
			}

			// Remove old mapping if device was already registered
			if existing, ok := r.clients[cfg.DeviceID]; ok {
				delete(r.byInbox, existing.InboxID)
			}

			client := NewClientFromConfig(cfg)
			r.clients[cfg.DeviceID] = client
			r.byInbox[cfg.InboxID] = cfg.DeviceID
			loaded++
		}
	})

	logrus.Infof("[CHATWOOT_REGISTRY] loaded %d clients (%d skipped/disabled)", loaded, skipped)
	return nil
}

// ListRegisteredDevices returns the list of device IDs that have registered Chatwoot clients.
func (r *ClientRegistry) ListRegisteredDevices() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	devices := make([]string, 0, len(r.clients))
	for deviceID := range r.clients {
		devices = append(devices, deviceID)
	}
	return devices
}

// IsConfigured returns true if a Chatwoot client is registered and configured for the given device.
func (r *ClientRegistry) IsConfigured(deviceID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	client, ok := r.clients[deviceID]
	return ok && client.IsConfigured()
}

// Global registry accessor for use by the whatsapp package.
var globalRegistry *ClientRegistry

// SetGlobalRegistry sets the global registry instance for package-level access.
func SetGlobalRegistry(registry *ClientRegistry) {
	globalRegistry = registry
}

// GetClientForDevice returns the Chatwoot client for a device via the global registry.
func GetClientForDevice(deviceID string) (*Client, error) {
	if globalRegistry == nil {
		return nil, fmt.Errorf("Chatwoot registry not initialized")
	}
	return globalRegistry.GetClient(deviceID)
}
