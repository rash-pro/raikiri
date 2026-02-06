# Raikiri

Raikiri is a unified stream chat aggregator designed for OBS overlays. It connects to multiple streaming platforms (Twitch, YouTube, Kick, and TikTok) and displays messages in a single, high-contrast, customizable web interface.

## Features

- **Multi-Platform Support**: Aggregates chat from Twitch, YouTube, Kick, and TikTok.
- **Unified Interface**: All messages appear in a single scrollable feed.
- **Glassmorphism Design**: Modern, semi-transparent UI that looks great as an OBS browser source.
- **High Contrast**: Optimized for readability on top of various video content.
- **Web Configuration**: Built-in settings modal to change channels without restarting the server manually.
- **Emote Support**: Renders emotes from all supported platforms.
- **Platform Badges**: Clearly identifies which platform each message came from (TW, YT, KI, TK).
- **Status Indicators**: Visual cues showing connection status for each service.

## Installation

### Prerequisites

- [Docker](https://www.docker.com/)
- [Docker Compose](https://docs.docker.com/compose/)

### Quick Start (Docker)

Run the following command in the project root to start Raikiri:

```bash
docker-compose up -d
```

Raikiri will be available at `http://localhost:30000`.

### Alternative Setup (Manual)

If you prefer not to use Docker, you will need [Node.js](https://nodejs.org/) installed:

1. **Install dependencies and setup environment**:
   - On **Linux/macOS**:
     ```bash
     chmod +x setup.sh && ./setup.sh
     ```
   - On **Windows**:
     ```batch
     setup.bat
     ```
2. **Start the server**:
   ```bash
   npm start
   ```

## Usage

1. Start Raikiri using the Docker command above.
2. Open your browser to `http://localhost:30000`.
3. To configure channels:
   - Hover over the bottom-left corner of the overlay to reveal the gear icon.
   - Click the gear to open the **Settings** modal.
4. Enter your channel handles (e.g., `ninja` for Twitch, `@user` for TikTok) and click **Save & Restart**.

### Adding to OBS

1. In OBS, add a new **Browser** source.
2. Set the URL to `http://localhost:30000`.
3. Set the width and height (e.g., 400x800).
4. (Optional) For monitoring or troubleshooting, use `http://localhost:30000/?debug=true` to show connection status indicators in the corner.

## License

This project is licensed under the **GNU General Public License v3.0**. See the [LICENSE](LICENSE) file for details.
