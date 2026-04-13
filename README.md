# Raikiri v2.0 - Streaming Interaction Hub

Raikiri is a high-performance, self-hosted streaming interaction hub built with **Bun**. It aggregates chat and events (Superchats, Subs, Bits, TikTok Gifts) from multiple platforms into a single unified overlay architecture with realistic Cloud-Synthesized TTS.

## Features
- **Multi-platform Hub**: Bi-directional communication with Twitch, YouTube Live, Kick, and TikTok Live.
- **High Performance**: Powered by Bun and embedded SQLite (6x faster than Node, 40% less memory).
- **Infinite Cloud TTS**: Microsoft Azure Neural TTS exposed via Edge APIs. Completely free, no quotas, and no GPU usage. Includes custom voice overrides per alert type (e.g. `es-MX-DaliaNeural` for bits, `ja-JP-NanamiNeural` for Twitch Custom Rewards).
- **EventSub & Advanced Alerts**: Subscribes dynamically to modern Twitch WebSockets. Fully themeable alerts with variables (e.g. `!voz` custom commands, Channel Point Redemptions).
- **OBS Local Media**: Store static media like `boom.gif` or `sound.mp3` in your `/data/media` volume without external third-party hosts.
- **Audio Client Tab**: Direct desktop audio output for OBS via a background browser tab (`/audio`), bypassing traditional Linux PulseAudio/ALSA capture issues with Browser Sources.
- **Sleek Overlays**: Fully responsive theming (`cyberpurple`, `ffvi`, etc) for `/overlay/chat/` and `/overlay/alerts/` tuned for OBS Browser Sources.
- **Admin Dashboard**: Manage accounts, trigger Twitch Device Code Auth securely without leaking tokens, and tune TTS settings on the fly. 

## Getting Started

1. **Deploy with Docker**:
```bash
docker-compose up -d --build
```
2. **Access Dashboard**:
Go to [http://localhost:30001/dashboard](http://localhost:30001/dashboard) in your browser.

3. **Connect Platforms**:
Fill out the Config and click Save. For Twitch OAuth, head to the Dashboard, enter your Client ID, and click "Authenticate Device". Enter the code at `twitch.tv/activate`.

4. **Local Media**:
Place any images, GIFs or MP3 SFX within `./data/media/` exactly where the `.sqlite` database lives. 
In the Raikiri dashboard, you can reference them simply by accessing the `/media/` path. For example: `/media/applause.mp3`.

5. **OBS Setup**:
Add Browser Sources in OBS pointing to:
- Chat Overlay: `http://localhost:30001/overlay/chat/` (suggested: 400x800)
- Alerts Overlay: `http://localhost:30001/overlay/alerts/` (suggested: 1920x1080)
- Audio (Open this in a normal background browser tab, NOT OBS): `http://localhost:30001/audio/` -> Press Enable Audio.

## Runtime Architecture (Bun)
Raikiri v2 uses standard standard ES modules and pure `bun` runtime APIs. 
To develop locally:
```bash
bun install
bun run dev
```
