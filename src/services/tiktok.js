const { WebcastPushConnection } = require('tiktok-live-connector');
const winston = require('winston');

class TikTokService {
    constructor(channelName, io, ignoredUsers = []) {
        this.channelName = channelName;
        this.io = io;
        this.ignoredUsers = ignoredUsers.map(u => u.toLowerCase());
        this.connection = null;

        this.logger = winston.createLogger({
            level: 'info',
            format: winston.format.combine(
                winston.format.timestamp(),
                winston.format.json()
            ),
            defaultMeta: { service: 'tiktok', channel: channelName },
            transports: [
                new winston.transports.Console({
                    format: winston.format.simple(),
                }),
            ],
        });
    }

    async connect() {
        try {
            this.emitStatus('connecting', `Connecting to TikTok live: ${this.channelName}...`);
            this.connection = new WebcastPushConnection(this.channelName);

            this.connection.connect().then(state => {
                this.logger.info(`Connected to TikTok room: ${state.roomId}`);
                this.emitStatus('connected');
            }).catch(err => {
                this.logger.error('TikTok connection failed:', err);
                this.emitStatus('error', err.message || 'Failed to connect');
            });

            this.connection.on('chat', (data) => {
                this.handleMessage(data);
            });

            this.connection.on('error', (err) => {
                this.logger.error('TikTok error:', err);
                this.emitStatus('error', 'Connection error');
            });

            this.connection.on('disconnected', () => {
                this.logger.warn('TikTok disconnected');
                this.emitStatus('disconnected');
            });

        } catch (error) {
            this.logger.error('Failed to initialize TikTok connection:', error);
            this.emitStatus('error', error.message);
        }
    }

    handleMessage(data) {
        // TikTok data format: { comment, userId, uniqueId, nickname, profilePictureUrl, badges, ... }
        if (this.ignoredUsers.includes(data.uniqueId.toLowerCase())) {
            return;
        }

        const message = {
            id: data.msgId,
            platform: 'tiktok',
            user: data.uniqueId,
            nickname: data.nickname,
            content: data.comment,
            color: '#ff0050', // TikTok Red/Pink
            isHtml: false,
            badges: this.normalizeBadges(data.badges),
            timestamp: new Date().toLocaleTimeString()
        };

        this.io.emit('chat_message', message);
    }

    normalizeBadges(badges) {
        const normalized = [];
        if (!badges) return normalized;

        // TikTok badges are complex, but we'll try to map common ones
        // common badges: badge_type: "moderator", "subscriber"
        badges.forEach(b => {
            const name = b.name ? b.name.toLowerCase() : '';
            if (name.includes('moderator')) normalized.push('moderator');
            if (name.includes('subscriber')) normalized.push('subscriber');
        });

        return normalized;
    }

    emitStatus(state, message = '') {
        this.io.emit('status', {
            platform: 'tiktok',
            state,
            message
        });

        this.io.emit('debug_log', {
            platform: 'tiktok',
            level: state === 'error' ? 'error' : 'info',
            message: message || `State changed to ${state}`
        });
    }

    disconnect() {
        if (this.connection) {
            this.connection.disconnect();
            this.connection = null;
        }
        this.emitStatus('disconnected');
    }
}

module.exports = TikTokService;
