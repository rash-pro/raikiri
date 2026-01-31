const Pusher = require('pusher-js');
const axios = require('axios');
const winston = require('winston');
const { exec } = require('child_process');
const util = require('util');
const execPromise = util.promisify(exec);

class KickService {
    constructor(channelName, io, ignoredUsers = []) {
        this.channelName = channelName;
        this.io = io;
        this.ignoredUsers = ignoredUsers.map(u => u.toLowerCase());
        this.pusher = null;
        this.chatroomId = null;
        this.channelId = null;

        this.logger = winston.createLogger({
            level: 'info',
            format: winston.format.combine(
                winston.format.timestamp(),
                winston.format.json()
            ),
            defaultMeta: { service: 'kick', channel: channelName },
            transports: [
                new winston.transports.Console({
                    format: winston.format.simple(),
                }),
            ],
        });
    }

    async connect() {
        try {
            this.emitStatus('connecting', `Fetching channel info for ${this.channelName}...`);

            // Using curl via child_process because Axios is getting blocked by Cloudflare WAF
            // even with browser headers. curl seems to bypass it more easily.
            const userAgent = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36';
            const url = `https://kick.com/api/v2/channels/${this.channelName}`;

            this.logger.info(`Fetching Kick metadata via curl for: ${this.channelName}`);
            const { stdout } = await execPromise(`curl -s -H "User-Agent: ${userAgent}" "${url}"`);

            const data = JSON.parse(stdout);

            if (data && data.chatroom) {
                this.chatroomId = data.chatroom.id;
                this.channelId = data.id;
                this.setupPusher();
            } else {
                throw new Error(data.message || 'Channel details not found');
            }
        } catch (error) {
            this.logger.error('Failed to connect to Kick:', {
                message: error.message,
                channel: this.channelName
            });
            this.emitStatus('error', `Failed to find channel: ${this.channelName}`);
        }
    }

    setupPusher() {
        this.pusher = new Pusher('32cbd69e4b950bf97679', {
            cluster: 'us2',
            wsHost: 'ws-us2.pusher.com',
            wsPort: 443,
            wssPort: 443,
            forceTLS: true,
            enabledTransports: ['ws', 'wss']
        });

        const channel = this.pusher.subscribe(`chatrooms.${this.chatroomId}.v2`);

        channel.bind('App\\Events\\ChatMessageEvent', (data) => {
            this.handleMessage(data);
        });

        this.pusher.connection.bind('connected', () => {
            this.logger.info(`Connected to Kick chat: ${this.channelName} (Chatroom ID: ${this.chatroomId})`);
            this.emitStatus('connected');
        });

        this.pusher.connection.bind('error', (err) => {
            this.logger.error('Pusher error:', err);
            // Don't emit error status immediately as Pusher might retry
            if (err.error && err.error.data && err.error.data.code === 4001) {
                this.emitStatus('error', 'Pusher cluster/key mismatch');
            }
        });
    }

    handleMessage(data) {
        // Normalize Kick message to Raikiri format
        // Kick data format example: { id, chatroom_id, content, type, created_at, sender: { id, username, slug, identity: { color, badges } } }

        const sender = data.sender;
        if (this.ignoredUsers.includes(sender.username.toLowerCase())) {
            this.logger.debug(`Ignored message from bot: ${sender.username}`);
            return;
        }

        const message = {
            id: data.id,
            platform: 'kick',
            user: sender.username,
            color: sender.identity.color || '#53fc18', // Default Kick green
            content: data.content,
            isHtml: true, // Kick messages can contain emotes
            badges: this.normalizeBadges(sender.identity.badges),
            timestamp: new Date().toLocaleTimeString()
        };

        // Parse emotes if any
        message.content = this.parseEmotes(message.content);

        this.io.emit('chat_message', message);
    }

    normalizeBadges(badges) {
        const normalized = [];
        if (!badges) return normalized;

        badges.forEach(b => {
            const type = b.type.toLowerCase();
            if (type === 'broadcaster' || type === 'owner') normalized.push('owner');
            else if (type === 'moderator') normalized.push('moderator');
            else if (type === 'vip') normalized.push('vip');
            else if (type === 'subscriber') normalized.push('subscriber');
        });

        return normalized;
    }

    parseEmotes(content) {
        // Kick emotes usually look like [emote:65432:emoteName]
        // Some also use :emoteName: but Kick API often returns the bracketed version.
        const emoteRegex = /\[emote:(\d+):([\w\d]+)\]/g;
        return content.replace(emoteRegex, (match, id, name) => {
            return `<img src="https://files.kick.com/emotes/${id}/fullsize" alt="${name}" title="${name}" class="emote">`;
        });
    }

    emitStatus(state, message = '') {
        this.io.emit('status', {
            platform: 'kick',
            state,
            message
        });

        this.io.emit('debug_log', {
            platform: 'kick',
            level: state === 'error' ? 'error' : 'info',
            message: message || `State changed to ${state}`
        });
    }

    disconnect() {
        if (this.pusher) {
            this.pusher.disconnect();
            this.pusher = null;
        }
        this.emitStatus('disconnected');
    }
}

module.exports = KickService;
