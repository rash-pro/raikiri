const fs = require('fs');
const path = require('path');
const winston = require('winston');

const CONFIG_PATH = path.join(__dirname, '../../config.json');

class ConfigManager {
    constructor() {
        this.config = {
            twitchChannels: [],
            youtube: {
                liveId: '',
                channelId: ''
            },
            kickChannel: '',
            tiktokChannel: '',
            ignoredUsers: []
        };
        this.load();
    }

    load() {
        try {
            // Load defaults from ENV
            if (process.env.TWITCH_CHANNELS) {
                this.config.twitchChannels = process.env.TWITCH_CHANNELS.split(',').map(c => c.trim());
            }

            if (process.env.YOUTUBE_LIVE_ID) {
                this.config.youtube.liveId = process.env.YOUTUBE_LIVE_ID;
            }
            if (process.env.YOUTUBE_CHANNEL_ID) {
                this.config.youtube.channelId = process.env.YOUTUBE_CHANNEL_ID;
            }

            if (process.env.KICK_CHANNEL) {
                this.config.kickChannel = process.env.KICK_CHANNEL;
            }

            if (process.env.TIKTOK_CHANNEL) {
                this.config.tiktokChannel = process.env.TIKTOK_CHANNEL;
            }

            if (process.env.IGNORED_USERS) {
                this.config.ignoredUsers = process.env.IGNORED_USERS.split(',').map(u => u.trim().toLowerCase());
            }

            // Override with file config
            if (fs.existsSync(CONFIG_PATH)) {
                const fileConfig = JSON.parse(fs.readFileSync(CONFIG_PATH, 'utf8'));
                this.config = { ...this.config, ...fileConfig };
                // Ensure nested youtube object is merged correctly if needed, but simple spread is ok if fileConfig has full structure
                // Ideally fileConfig should match structure
                if (fileConfig.youtube) {
                    this.config.youtube = { ...this.config.youtube, ...fileConfig.youtube };
                }
            }
        } catch (error) {
            console.error('Error loading config:', error);
        }
    }

    save(newConfig) {
        try {
            this.config = { ...this.config, ...newConfig };
            fs.writeFileSync(CONFIG_PATH, JSON.stringify(this.config, null, 2));
            return true;
        } catch (error) {
            console.error('Error saving config:', error);
            return false;
        }
    }

    get() {
        return this.config;
    }
}

module.exports = new ConfigManager();
