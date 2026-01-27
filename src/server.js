const express = require('express');
const http = require('http');
const { Server } = require('socket.io');
const path = require('path');
const TwitchService = require('./services/twitch');
const YouTubeService = require('./services/youtube');
const LogBuffer = require('./logBuffer');
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

// Config
const ignoredUsers = process.env.IGNORED_USERS ? process.env.IGNORED_USERS.split(',').map(u => u.trim().toLowerCase()) : [];

if (process.env.TWITCH_CHANNELS) {
    const channels = process.env.TWITCH_CHANNELS.split(',');
    const twitchService = new TwitchService(channels, io, ignoredUsers);
    twitchService.connect();
    services.push(twitchService);
    logger.info(`Initialized Twitch service for: ${channels.join(', ')}`);
}

// YouTube Service Init
let ytIdentifier = {};
if (process.env.YOUTUBE_LIVE_ID) {
    ytIdentifier = { liveId: process.env.YOUTUBE_LIVE_ID };
} else if (process.env.YOUTUBE_CHANNEL_ID) {
    ytIdentifier = { channelId: process.env.YOUTUBE_CHANNEL_ID };
} else if (process.env.YOUTUBE_ID) {
    // Fallback for legacy or loose config
    ytIdentifier = { liveId: process.env.YOUTUBE_ID };
}

if (ytIdentifier.liveId || ytIdentifier.channelId) {
    const youtubeService = new YouTubeService(ytIdentifier, io, ignoredUsers);
    youtubeService.connect();
    services.push(youtubeService);
    logger.info(`Initialized YouTube service`);
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

    // Send public config
    socket.emit('config', {
        twitchChannels: process.env.TWITCH_CHANNELS ? process.env.TWITCH_CHANNELS.split(',') : [],
        youtubeId: process.env.YOUTUBE_LIVE_ID || process.env.YOUTUBE_CHANNEL_ID || ''
    });

    // Replay logs for debug
    LogBuffer.replay(socket);

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
