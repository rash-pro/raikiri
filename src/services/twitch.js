const tmi = require('tmi.js');
const winston = require('winston');

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
                content: message,
                color: tags['color'],
                timestamp: new Date().toISOString(),
                badges: tags['badges-raw'],
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
