const tmi = require('tmi.js');
const winston = require('winston');
const { escapeHtml } = require('../utils/sanitizer');

class TwitchService {
    constructor(channels, io) {
        this.io = io;
        this.client = new tmi.Client({
            connection: {
                secure: true,
                reconnect: true,
            },
            channels: channels,
        });

        this.logger = winston.createLogger({
            level: 'info',
            defaultMeta: { service: 'twitch' },
            transports: [new winston.transports.Console({ format: winston.format.simple() })]
        });

        this.setupListeners();
    }

    setupListeners() {
        this.client.on('message', (channel, tags, message, self) => {
            // Ignore self-messages (though unlikely with anonymous)
            if (self) return;

            const chatMessage = {
                platform: 'twitch',
                id: tags['id'],
                user: tags['display-name'] || tags['username'],
                content: this.parseMessage(message, tags.emotes), // Parse emotes
                color: tags['color'],
                timestamp: new Date().toISOString(),
                badges: tags['badges-raw'],
                isHtml: true // Flag to tell client to render as HTML
            };

            this.io.emit('chat_message', chatMessage);
            // this.logger.info(`[${channel}] ${chatMessage.user}: ${chatMessage.content}`);
        });

        this.client.on('connected', (address, port) => {
            this.logger.info(`Connected to ${address}:${port}`);
            this.io.emit('status', { platform: 'twitch', state: 'connected' });
        });

        this.client.on('disconnected', (reason) => {
            this.logger.warn(`Disconnected: ${reason}`);
            this.io.emit('status', { platform: 'twitch', state: 'disconnected', message: reason });
        });
    }

    parseMessage(message, emotes) {
        // If no emotes, just escape the text
        if (!emotes) return escapeHtml(message);

        // Create an array of replacements: { start, end, id }
        const replacements = [];
        Object.keys(emotes).forEach(id => {
            emotes[id].forEach(range => {
                const [start, end] = range.split('-').map(Number);
                replacements.push({ start, end, id });
            });
        });

        // Sort by start index desc to replace from end to start without messing up indices
        replacements.sort((a, b) => b.start - a.start);

        let htmlMessage = message;

        // We need to carefully handle mixing escaped text and HTML tags.
        // Strategy: Break string into segments, escape text segments, and insert image tags for emote segments.
        // Actually, simpler: Since we replace from end, we can cut the string.

        // However, we must escape the text *around* the emotes.
        // So we rebuild the string.

        // Better approach with existing code structure:
        // 1. Identify all emote ranges.
        // 2. Identify all non-emote text ranges.
        // 3. Escape text ranges.
        // 4. Construct HTML.

        // Refined approach (End-to-Start):
        // We iterate replacements. For each replacement, we grab the text *after* it (up to previous replacement's start),
        // escape it, prepend it to our accumulator. Then prepend the emote img.
        // Finally prepend the text *before* the first replacement (escaped).

        let result = "";
        let lastIndex = message.length;

        replacements.forEach(rep => {
            // Text after this emote
            const tail = message.substring(rep.end + 1, lastIndex);
            result = escapeHtml(tail) + result;

            // The Emote
            const emoteUrl = `https://static-cdn.jtvnw.net/emoticons/v2/${rep.id}/default/dark/1.0`;
            const imgTag = `<img src="${emoteUrl}" class="emote" alt="emote">`;
            result = imgTag + result;

            lastIndex = rep.start;
        });

        // Text before first emote
        const head = message.substring(0, lastIndex);
        result = escapeHtml(head) + result;

        return result;
    }

    async connect() {
        try {
            await this.client.connect();
        } catch (err) {
            this.logger.error('Failed to connect to Twitch', err);
            this.io.emit('status', { platform: 'twitch', state: 'error', message: 'Failed to connect' });
        }
    }

    async disconnect() {
        try {
            await this.client.disconnect();
        } catch (err) {
            this.logger.error('Error disconnecting from Twitch', err);
        }
    }
}

module.exports = TwitchService;
