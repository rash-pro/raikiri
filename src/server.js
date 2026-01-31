const express = require('express');
const http = require('http');
const { Server } = require('socket.io');
const path = require('path');
const TwitchService = require('./services/twitch');
const YouTubeService = require('./services/youtube');
const LogBuffer = require('./logBuffer');
const winston = require('winston');
const configManager = require('./utils/configManager');

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

// Middleware
app.use(express.static(path.join(__dirname, 'public')));
app.use(express.json());

// Services Management
let services = [];

/**
 * Stop and clear all running services
 */
const stopServices = () => {
    logger.info('Stopping services...');
    services.forEach(service => {
        if (service.disconnect) service.disconnect();
    });
    services = [];
};

/**
 * Start services based on current config
 */
const startServices = () => {
    stopServices(); // Ensure clean slate
    const config = configManager.get();

    // Twitch
    if (config.twitchChannels && config.twitchChannels.length > 0) {
        // filter out empty strings
        const channels = config.twitchChannels.filter(c => c && c.length > 0);
        if (channels.length > 0) {
            const twitchService = new TwitchService(channels, io, config.ignoredUsers);
            twitchService.connect();
            services.push(twitchService);
            logger.info(`Initialized Twitch service for: ${channels.join(', ')}`);
        }
    }

    // YouTube
    const ytIdentifier = {};
    if (config.youtube.liveId) ytIdentifier.liveId = config.youtube.liveId;
    if (config.youtube.channelId) ytIdentifier.channelId = config.youtube.channelId;

    if (ytIdentifier.liveId || ytIdentifier.channelId) {
        const youtubeService = new YouTubeService(ytIdentifier, io, config.ignoredUsers);
        youtubeService.connect();
        services.push(youtubeService);
        logger.info(`Initialized YouTube service`);
    } else {
        logger.info('No YouTube configuration found.');
    }
};

// Initial Start
startServices();

// Routes
app.get('/', (req, res) => {
    res.sendFile(path.join(__dirname, 'public', 'index.html'));
});

// API Routes
app.get('/api/config', (req, res) => {
    res.json(configManager.get());
});

app.post('/api/config', (req, res) => {
    try {
        const newConfig = req.body;
        logger.info('Received new config:', newConfig);

        if (configManager.save(newConfig)) {
            startServices(); // Restart with new config
            // Broadcast new config to all clients so they can update UI if needed
            io.emit('config', {
                twitchChannels: newConfig.twitchChannels || [],
                youtubeId: newConfig.youtube.liveId || newConfig.youtube.channelId || ''
            });
            res.json({ success: true, message: 'Configuration saved and services restarted.' });
        } else {
            res.status(500).json({ success: false, message: 'Failed to save configuration.' });
        }
    } catch (error) {
        logger.error('Error in POST /api/config:', error);
        res.status(500).json({ success: false, message: error.message });
    }
});

// Socket.io
io.on('connection', (socket) => {
    logger.info('A client connected');

    // Send public config
    const config = configManager.get();
    socket.emit('config', {
        twitchChannels: config.twitchChannels || [],
        youtubeId: config.youtube.liveId || config.youtube.channelId || ''
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

module.exports = { app, server };
