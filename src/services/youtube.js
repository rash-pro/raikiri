const { LiveChat } = require('youtube-chat');
const winston = require('winston');

const LogBuffer = require('../logBuffer');
const { escapeHtml } = require('../utils/sanitizer');

class YouTubeService {
    constructor(identifier, io) {
        this.io = io;
        // Identifier can be channelId or liveId. 
        // For simplicity in this version, we will assume it might be passed as { channelId: '...' } or { liveId: '...' }
        // If it's a string, we'll try to guess or let the user specify in the config later.
        // For now, let's assume the constructor receives an object or string.

        this.liveChat = new LiveChat(identifier);

        this.logger = winston.createLogger({
            level: 'debug',
            defaultMeta: { service: 'youtube' },
            transports: [new winston.transports.Console({ format: winston.format.simple() })]
        });

        this.setupListeners();
    }

    logToClient(level, message, details = null) {
        const log = {
            platform: 'youtube',
            level,
            message,
            details,
            timestamp: new Date().toISOString()
        };

        // Add to buffer
        LogBuffer.add(log);

        // Emit to all
        this.io.emit('debug_log', log);
    }

    setupListeners() {
        this.liveChat.on('chat', (chatItem) => {
            const chatMessage = {
                platform: 'youtube',
                id: chatItem.id,
                user: chatItem.author.name,
                // YouTube chat items have 'message' which is an array of runs (text/emoji)
                // We need to convert it to string for simple display
                content: this.parseMessage(chatItem.message),
                color: null, // YouTube doesn't expose user color in the same way
                timestamp: new Date(chatItem.timestamp).toISOString(),
                authorDetails: chatItem.author, // Extra details if needed
                isHtml: true
            };

            this.io.emit('chat_message', chatMessage);
        });

        this.liveChat.on('start', (liveId) => {
            this.logger.info(`Connected to YouTube stream: ${liveId}`);
            this.logToClient('info', `Connected to stream: ${liveId}`);
            this.io.emit('status', { platform: 'youtube', state: 'connected' });
        });

        this.liveChat.on('error', (err) => {
            this.logger.error('YouTube Chat Error', err);
            this.logToClient('error', 'Connection Error', err.message);
            this.io.emit('status', { platform: 'youtube', state: 'error', message: err.message });
        });

        this.liveChat.on('end', (reason) => {
            this.logger.info(`YouTube stream ended: ${reason}`);
            this.logToClient('warn', `Stream ended: ${reason}`);
            this.io.emit('status', { platform: 'youtube', state: 'disconnected', message: reason });
        });
    }

    parseMessage(messageRuns) {
        if (!Array.isArray(messageRuns)) return '';

        return messageRuns.map(run => {
            // Case 1: Nested emoji object (Legacy/Standard)
            if (run.emoji) {
                const url = run.emoji.image?.thumbnails?.[0]?.url;
                if (url) return `<img src="${url}" class="emote" alt="${run.emoji.emojiId || 'emote'}">`;
                return run.text || run.emoji.shortcuts?.[0] || '';
            }

            // Case 2: Flat emoji object (Confirmed by User Logs)
            if (run.url) {
                return `<img src="${run.url}" class="emote" alt="${run.alt || 'emote'}">`;
            }

            // Case 3: Text
            return escapeHtml(run.text || '');
        }).join('');
    }

    async connect() {
        try {
            this.logToClient('info', 'Attempting to connect...');
            const ok = await this.liveChat.start();
            if (!ok) {
                this.logger.error('Failed to start YouTube chat listener');
                this.logToClient('error', 'Failed to start listener');
                this.io.emit('status', { platform: 'youtube', state: 'error', message: 'Failed to start' });
            }
        } catch (err) {
            this.logger.error('Failed to connect to YouTube', err);
            this.logToClient('error', 'Connection Exception', err.message);
            this.io.emit('status', { platform: 'youtube', state: 'error', message: err.message });
        }
    }

    async disconnect() {
        this.liveChat.stop();
    }
}

module.exports = YouTubeService;
