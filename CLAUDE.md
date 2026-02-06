# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Raikiri is a unified stream chat aggregator designed for OBS overlays that connects multiple streaming platforms (Twitch, YouTube, Kick, TikTok) and displays messages in a single, customizable web interface. It uses a service-based architecture where each platform has its own service class.

## Architecture

### Backend Structure

- **Entry Point**: `src/server.js` - Express server with Socket.io
- **Services**: `src/services/` - Platform-specific streaming services
  - `twitch.js` - Uses tmi.js library for Twitch chat
  - `youtube.js` - Uses youtube-chat library
  - `kick.js` - Uses Pusher library via HTTP/curl to bypass Cloudflare
  - `tiktok.js` - Uses built-in tiktok-live-connector
- **Utilities**: `src/utils/` - Configuration management and sanitization
  - `configManager.js` - Reads config from ENV vars or `config.json`
  - `sanitizer.js` - HTML escaping for security

### Service Interface Pattern

All services follow a consistent interface:
- Constructor takes: `channels/channelName`, `io`, `ignoredUsers` array
- `connect()` - Starts the streaming service
- `disconnect()` - Stops the service cleanly
- `setupListeners()` - Sets up events (message, status, error)

### Communication Flow

1. Server starts → checks config → instantiates enabled services
2. Services connect to their platforms (WS/HTTP)
3. Services emit standardized events via Socket.io
4. Client receives `chat_message`, `status`, and `debug_log` events

## Commands

- **Quick Start (Interactive)**:
  - Linux/macOS: `chmod +x setup.sh && ./setup.sh`
  - Windows: `setup.bat`
  - This prompts for Twitch channels, YouTube video ID/channel ID, and ignored users
  - Creates `.env` file and starts Docker container automatically

- **Start Server**: `npm start` (recommended for development)

- **Run Tests**: `jest`

- **Docker**: `docker-compose up -d` (manual start)

- **Docker Compose**: `docker-compose up --build -d` (for setup.sh/setup.bat)

The default service port is 30000 (configurable via `PORT` env var).

## Configuration

Configuration is managed by `src/utils/configManager.js`:
- Priority: ENV variables → `config.json`
- ENV variables:
  - `TWITCH_CHANNELS` - Comma-separated list of channel usernames
  - `YOUTUBE_LIVE_ID` - YouTube live stream ID
  - `YOUTUBE_CHANNEL_ID` - YouTube channel ID
  - `KICK_CHANNEL` - Kick channel username
  - `TIKTOK_CHANNEL` - TikTok username
  - `IGNORED_USERS` - Comma-separated users to filter out
- Configuration changes through `/api/config` endpoint trigger hot-restart via `startServices()` in server.js

## Development Notes

### Adding a New Platform Service

When adding a new platform:
1. Create `src/services/platformName.js`
2. Implement the service class following the established pattern
3. Update `src/server.js` to instantiate the new service when configured
4. Add platform badge display in `src/public/client.js`

### Emote Handling

- Twitch: Parse Twitch emote tokens and render via CDN URLs
- YouTube: Parse message runs and render emoji objects
- Kick: Parse emote tokens and render via Kick CDN
- TikTok: Currently displays text-only (platform badge color is pink)

### Logging

- Services use Winston with service-specific loggers
- All platforms emit `debug_log` for debug panel (access via `/?debug=true`)
- Background logs are buffered in `src/logBuffer.js` (replay functionality)

### Security

Always use `escapeHtml()` from `src/utils/sanitizer.js` when rendering user-generated content to prevent XSS. The `isHtml` flag in chat messages indicates whether content should be rendered as HTML (safe, pre-escaped) or text.
