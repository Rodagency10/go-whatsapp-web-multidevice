export default {
    name: 'ChatwootManager',
    props: {
        selectedDeviceId: {
            type: String,
            default: ''
        }
    },
    data() {
        return {
            config: null,
            form: {
                chatwoot_url: '',
                api_token: '',
                account_id: '',
                inbox_id: '',
                enabled: true
            },
            syncForm: {
                days_limit: 3,
                include_media: true,
                include_groups: true
            },
            loading: false,
            saving: false,
            syncing: false,
            syncProgress: null,
            showForm: false,
            showSyncModal: false
        }
    },
    computed: {
        isConfigured() {
            return this.config && this.config.is_configured;
        },
        canSync() {
            return this.isConfigured && !this.syncing;
        }
    },
    watch: {
        selectedDeviceId() {
            this.fetchConfig();
            this.resetForm();
        }
    },
    methods: {
        resetForm() {
            this.form = {
                chatwoot_url: '',
                api_token: '',
                account_id: '',
                inbox_id: '',
                enabled: true
            };
            this.showForm = false;
        },
        async fetchConfig() {
            if (!this.selectedDeviceId) return;
            try {
                this.loading = true;
                const res = await window.http.get(`/devices/${encodeURIComponent(this.selectedDeviceId)}/chatwoot`);
                this.config = res.data.results || null;

                if (this.config && this.config.is_configured) {
                    this.form.chatwoot_url = this.config.chatwoot_url || '';
                    this.form.account_id = this.config.account_id || '';
                    this.form.inbox_id = this.config.inbox_id || '';
                    this.form.enabled = this.config.enabled !== false;
                }
            } catch (err) {
                console.error(err);
            } finally {
                this.loading = false;
            }
        },
        async saveConfig() {
            if (!this.selectedDeviceId) {
                showErrorInfo('Please select a device first');
                return;
            }
            if (!this.form.chatwoot_url) {
                showErrorInfo('Chatwoot URL is required');
                return;
            }
            if (!this.form.api_token && !this.isConfigured) {
                showErrorInfo('API Token is required');
                return;
            }
            if (!this.form.account_id) {
                showErrorInfo('Account ID is required');
                return;
            }
            if (!this.form.inbox_id) {
                showErrorInfo('Inbox ID is required');
                return;
            }

            try {
                this.saving = true;
                const payload = {
                    chatwoot_url: this.form.chatwoot_url,
                    account_id: parseInt(this.form.account_id),
                    inbox_id: parseInt(this.form.inbox_id),
                    enabled: this.form.enabled
                };
                if (this.form.api_token) {
                    payload.api_token = this.form.api_token;
                }

                await window.http.put(`/devices/${encodeURIComponent(this.selectedDeviceId)}/chatwoot`, payload);
                showSuccessInfo('Chatwoot configuration saved');
                this.showForm = false;
                await this.fetchConfig();
            } catch (err) {
                const msg = err.response?.data?.message || err.message || 'Failed to save configuration';
                showErrorInfo(msg);
            } finally {
                this.saving = false;
            }
        },
        async deleteConfig() {
            if (!this.selectedDeviceId) return;

            $('#deleteChatwootConfigModal').modal({
                closable: false,
                onApprove: async () => {
                    try {
                        await window.http.delete(`/devices/${encodeURIComponent(this.selectedDeviceId)}/chatwoot`);
                        showSuccessInfo('Chatwoot configuration deleted');
                        this.config = null;
                        this.resetForm();
                    } catch (err) {
                        const msg = err.response?.data?.message || err.message || 'Failed to delete configuration';
                        showErrorInfo(msg);
                    }
                    return false;
                }
            }).modal('show');
        },
        async startSync() {
            if (!this.selectedDeviceId) return;

            try {
                this.syncing = true;
                const payload = {
                    days_limit: this.syncForm.days_limit,
                    include_media: this.syncForm.include_media,
                    include_groups: this.syncForm.include_groups
                };

                await window.http.post(`/devices/${encodeURIComponent(this.selectedDeviceId)}/chatwoot/sync`, payload);
                showSuccessInfo('History sync started in background');
                $('#syncHistoryModal').modal('hide');
                this.checkSyncStatus();
            } catch (err) {
                const msg = err.response?.data?.message || err.message || 'Failed to start sync';
                showErrorInfo(msg);
            } finally {
                this.syncing = false;
            }
        },
        async checkSyncStatus() {
            if (!this.selectedDeviceId) return;

            try {
                const res = await window.http.get(`/devices/${encodeURIComponent(this.selectedDeviceId)}/chatwoot/sync/status`);
                const result = res.data.results;
                if (result && result.status) {
                    this.syncProgress = result;
                    if (result.status === 'running') {
                        setTimeout(() => this.checkSyncStatus(), 3000);
                    }
                }
            } catch (err) {
                console.error('Failed to check sync status:', err);
            }
        },
        openSyncModal() {
            this.syncProgress = null;
            $('#syncHistoryModal').modal('show');
        },
        openConfigModal() {
            this.showForm = true;
        }
    },
    mounted() {
        this.fetchConfig();
    },
    template: `
    <div class="ui segment">
        <h3 class="ui header">
            <i class="comments outline icon"></i>
            <div class="content">
                Chatwoot Integration
                <div class="sub header">Configure Chatwoot inbox for this device</div>
            </div>
        </h3>

        <div v-if="loading" class="ui active inline loader"></div>

        <div v-if="!loading && !isConfigured" class="ui message info">
            <div class="header">Not configured</div>
            <p>Configure Chatwoot to receive WhatsApp messages in your support inbox.</p>
            <button class="ui primary button" @click="openConfigModal">
                <i class="plus icon"></i>Configure Chatwoot
            </button>
        </div>

        <div v-if="!loading && isConfigured">
            <div class="ui list">
                <div class="item">
                    <i class="check circle green icon"></i>
                    <div class="content">
                        <div class="header">Configured</div>
                        <div class="description">
                            Account: {{ config.account_id }} · Inbox: {{ config.inbox_id }}
                        </div>
                    </div>
                </div>
                <div class="item">
                    <i class="linkify icon"></i>
                    <div class="content">
                        <div class="description">{{ config.chatwoot_url }}</div>
                    </div>
                </div>
                <div class="item">
                    <i :class="config.enabled ? 'toggle on green icon' : 'toggle off red icon'"></i>
                    <div class="content">
                        <div class="description">{{ config.enabled ? 'Enabled' : 'Disabled' }}</div>
                    </div>
                </div>
            </div>

            <div class="ui buttons">
                <button class="ui button" @click="openConfigModal">
                    <i class="edit icon"></i>Edit
                </button>
                <div class="or"></div>
                <button class="ui blue button" :disabled="!canSync" @click="openSyncModal">
                    <i class="sync icon"></i>Sync History
                </button>
                <div class="or"></div>
                <button class="ui red button" @click="deleteConfig">
                    <i class="trash icon"></i>Delete
                </button>
            </div>

            <div v-if="syncProgress" class="ui progress active" :class="syncProgress.status === 'completed' ? 'success' : syncProgress.status === 'failed' ? 'error' : ''">
                <div class="bar" :style="'width: ' + (syncProgress.total_messages > 0 ? Math.round((syncProgress.synced_messages / syncProgress.total_messages) * 100) : 0) + '%'">
                    <div class="progress"></div>
                </div>
                <div class="label">
                    {{ syncProgress.status }} - {{ syncProgress.synced_messages }}/{{ syncProgress.total_messages }} messages
                </div>
            </div>
        </div>

        <!-- Configuration Modal -->
        <div class="ui modal" id="chatwootConfigModal" :class="{active: showForm}">
            <div class="header">
                <i class="comments outline icon"></i>
                {{ isConfigured ? 'Edit' : 'Configure' }} Chatwoot
            </div>
            <div class="content">
                <div class="ui form">
                    <div class="field">
                        <label>Chatwoot URL</label>
                        <input type="url" v-model="form.chatwoot_url" placeholder="https://app.chatwoot.com">
                    </div>
                    <div class="field">
                        <label>API Token</label>
                        <input type="password" v-model="form.api_token" :placeholder="isConfigured ? 'Leave blank to keep current' : 'Your Chatwoot API token'">
                    </div>
                    <div class="two fields">
                        <div class="field">
                            <label>Account ID</label>
                            <input type="number" v-model="form.account_id" placeholder="1">
                        </div>
                        <div class="field">
                            <label>Inbox ID</label>
                            <input type="number" v-model="form.inbox_id" placeholder="1">
                        </div>
                    </div>
                    <div class="field">
                        <div class="ui toggle checkbox" :class="{checked: form.enabled}">
                            <input type="checkbox" v-model="form.enabled">
                            <label>Enabled</label>
                        </div>
                    </div>
                </div>
                <div class="ui warning message">
                    <div class="header">How to get these values</div>
                    <ol>
                        <li><strong>URL</strong>: Your Chatwoot instance URL (e.g., https://app.chatwoot.com)</li>
                        <li><strong>API Token</strong>: Settings → Profile Settings → Access Token</li>
                        <li><strong>Account ID</strong>: Visible in URL: /app/accounts/<strong>ID</strong>/dashboard</li>
                        <li><strong>Inbox ID</strong>: Settings → Inboxes → Select API inbox → ID in URL</li>
                    </ol>
                </div>
            </div>
            <div class="actions">
                <button class="ui cancel button">Cancel</button>
                <button class="ui primary button" :class="{loading: saving}" @click="saveConfig">
                    <i class="save icon"></i>Save
                </button>
            </div>
        </div>

        <!-- Sync History Modal -->
        <div class="ui modal" id="syncHistoryModal">
            <div class="header">
                <i class="sync icon"></i>
                Sync Message History
            </div>
            <div class="content">
                <div class="ui form">
                    <div class="field">
                        <label>Days of history to sync</label>
                        <input type="number" v-model="syncForm.days_limit" min="1" max="90">
                    </div>
                    <div class="field">
                        <div class="ui checkbox" :class="{checked: syncForm.include_media}">
                            <input type="checkbox" v-model="syncForm.include_media">
                            <label>Include media attachments</label>
                        </div>
                    </div>
                    <div class="field">
                        <div class="ui checkbox" :class="{checked: syncForm.include_groups}">
                            <input type="checkbox" v-model="syncForm.include_groups">
                            <label>Include group chats</label>
                        </div>
                    </div>
                </div>
                <div v-if="syncProgress" class="ui progress active" :class="syncProgress.status === 'completed' ? 'success' : syncProgress.status === 'failed' ? 'error' : ''">
                    <div class="bar" :style="'width: ' + (syncProgress.total_messages > 0 ? Math.round((syncProgress.synced_messages / syncProgress.total_messages) * 100) : 0) + '%'">
                        <div class="progress"></div>
                    </div>
                    <div class="label">
                        {{ syncProgress.status }} - {{ syncProgress.synced_messages }}/{{ syncProgress.total_messages }} messages
                    </div>
                </div>
            </div>
            <div class="actions">
                <button class="ui cancel button">Close</button>
                <button class="ui primary button" :class="{loading: syncing}" :disabled="syncProgress && syncProgress.status === 'running'" @click="startSync">
                    <i class="sync icon"></i>Start Sync
                </button>
            </div>
        </div>

        <!-- Delete Confirmation Modal -->
        <div class="ui modal" id="deleteChatwootConfigModal">
            <div class="header">
                <i class="trash alternate icon"></i>
                Delete Chatwoot Configuration
            </div>
            <div class="content">
                <p>Are you sure you want to delete the Chatwoot configuration for this device?</p>
                <div class="ui warning message">
                    <div class="header">Warning</div>
                    <p>This will stop forwarding messages to Chatwoot and remove the client configuration. This cannot be undone.</p>
                </div>
            </div>
            <div class="actions">
                <button class="ui cancel button">Cancel</button>
                <button class="ui red approve button">
                    <i class="trash icon"></i>Delete
                </button>
            </div>
        </div>
    </div>
    `
}
