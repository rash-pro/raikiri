const express = require('express');
const http = require('http');
const { Server } = require('socket.io');
const path = require('path');
const TwitchService = require('./services/twitch');
const YouTubeService = require('./services/youtube');
const winston = require('winston');

// Logger setup
const logger = winston.createLogger({
    level: 'info',
    format: winston.format.combine(
        winston.format.timestamp(),
        winston.format.json()
    ),
    transports: [
        new winston.transports.Console({
            format: winston.format.simple(),
        }),
    ],
});

const app = express();
const server = http.createServer(app);
const io = new Server(server);

// Services
const services = [];

if (process.env.TWITCH_CHANNELS) {
    const channels = process.env.TWITCH_CHANNELS.split(',');
    const twitchService = new TwitchService(channels, io);
    twitchService.connect();
    services.push(twitchService);
    logger.info(`Initialized Twitch service for: ${channels.join(', ')}`);
}

if (process.env.YOUTUBE_ID) {
    // try to determine if it's a channel or video ID based on length or prefix, 
    // or just let the library handle it if we pass the right object.
    // simpler: assume YOUTUBE_ID can be a channelId or liveId. 
    // youtube-chat supports { channelId: '...' } or { liveId: '...' }
    // We will simple pass the string if it looks like a video ID, or object if it looks like a channel.
    // For now, let's assume it's a Live Video ID if it's short, or we can use another ENV for type.

    // Better approach: Separate params
    let identifier = {};
    if (process.env.YOUTUBE_LIVE_ID) {
        identifier = { liveId: process.env.YOUTUBE_LIVE_ID };
    } else if (process.env.YOUTUBE_CHANNEL_ID) {
        identifier = { channelId: process.env.YOUTUBE_CHANNEL_ID };
    }

    if (identifier.liveId || identifier.channelId) {
        const youtubeService = new YouTubeService(identifier, io);
        youtubeService.connect();
        services.push(youtubeService);
        logger.info(`Initialized YouTube service`);
    }
}

// Middleware
app.use(express.static(path.join(__dirname, 'public')));

// Routes
app.get('/', (req, res) => {
    res.sendFile(path.join(__dirname, 'public', 'index.html'));
});

// Socket.io
io.on('connection', (socket) => {
    logger.info('A client connected');
    socket.on('disconnect', () => {
        logger.info('A client disconnected');
    });
});

// Start server
const PORT = process.env.PORT || 30000;
server.listen(PORT, () => {
    logger.info(`Server running on port ${PORT}`);
});

module.exports = { app, server }; // Export for testing
