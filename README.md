# Raikiri Native

Raikiri is a self-hosted streaming interaction hub for OBS. It aggregates live chat and stream events into local browser overlays, alert scenes, and browser-based TTS audio.

This is the native Go rewrite. It ships as a small standalone binary. If you need the previous Bun/Docker implementation, it remains available at tag `2.1.2`.

## Version Line

- **Current line:** Raikiri Native, built in Go.
- **Previous line:** the Bun/Docker implementation remains available at tag `2.1.2`.
- **Phase 1 native platforms:** Twitch and YouTube Live.
- **Phase 2 planned platforms:** Kick and TikTok. They are intentionally disabled in the native dashboard until their adapters are implemented.

## What You Get

- Native executable for Linux, Windows, and macOS.
- Embedded dashboard and OBS overlay assets.
- Local SQLite config/database under your chosen data directory.
- Twitch chat, Twitch EventSub alerts, YouTube Live chat via web-first polling.
- Edge TTS cloud voices streamed to `/audio`.
- Local media support via `/media/*`.
- OBS browser sources for chat, alerts, widgets, and audio.
- Configurable widgets for support goals, recent events, and user-defined custom overlays.

## Downloading a Release

Download the binary for your OS from the release artifacts:

- Linux x64: `raikiri-linux-amd64`
- Linux ARM64: `raikiri-linux-arm64`
- Windows x64: `raikiri-windows-amd64.exe`
- macOS Apple Silicon: `raikiri-darwin-arm64`
- macOS Intel: `raikiri-darwin-amd64`

Optional: verify checksums with `SHA256SUMS`.

## Running

### Linux

```bash
chmod +x ./raikiri-linux-amd64
./raikiri-linux-amd64 serve --host 127.0.0.1 --port 30001 --data-dir ./data
```

### macOS

```bash
chmod +x ./raikiri-darwin-arm64
./raikiri-darwin-arm64 serve --host 127.0.0.1 --port 30001 --data-dir ./data
```

If macOS Gatekeeper blocks the binary, allow it in System Settings or remove quarantine for a trusted local build:

```bash
xattr -d com.apple.quarantine ./raikiri-darwin-arm64
```

### Windows PowerShell

```powershell
.\raikiri-windows-amd64.exe serve --host 127.0.0.1 --port 30001 --data-dir .\data
```

Then open:

```text
http://localhost:30001/dashboard/
```

## First Setup

1. Start Raikiri with one of the commands above.
2. Open `http://localhost:30001/dashboard/`.
3. Configure Twitch and/or YouTube.
4. Click **Save Configuration**.
5. Add the OBS browser sources listed below.

### Twitch

For Twitch chat only, set **Channel Username** in the dashboard and save.

For Twitch EventSub alerts, create a Twitch developer app, copy its **Client ID**, save it in the dashboard, then click **Authenticate Device**. Open the activation link and enter the displayed code. Raikiri stores the token locally in SQLite under `--data-dir`.

### YouTube

Set **Channel ID**, **Video ID**, or handle-like live target in the YouTube field and save.

The native adapter is web-first: it follows YouTube's public live page and Innertube continuation flow, similar to the previous JavaScript implementation. It does not require a YouTube API key in Phase 1.

### Kick and TikTok

Kick and TikTok are not implemented in this native version yet. The dashboard shows them as `Phase 2` and disables their inputs.

## OBS Sources

Add these as OBS Browser Sources:

- Chat overlay: `http://localhost:30001/overlay/chat/`
- Alerts overlay: `http://localhost:30001/overlay/alerts/`
- Support goal widget: `http://localhost:30001/overlay/widgets/support-goal/`
- Recent events widget: `http://localhost:30001/overlay/widgets/recent-events/`
- Custom widget: `http://localhost:30001/overlay/widgets/custom/?id=YOUR_WIDGET_ID`
- Audio output: `http://localhost:30001/audio/`

Suggested sizes:

- Chat: `400x800`, or your preferred chat column size.
- Alerts: `1920x1080`.
- Widgets: match the widget width configured in the dashboard, or use a transparent `1920x1080` source.
- Audio: any size; enable **Control audio via OBS** if desired.

If OBS or the browser blocks autoplay on `/audio`, right-click the source, choose **Interact**, and press **Enable Audio** once.

## Widgets

Raikiri includes a widget system for OBS browser sources. Widgets use `/api/widgets/state` for initial state and `/ws/widgets` for live updates.

Built-in widgets:

- **Support Goal:** tracks support events since the last reset.
- **Recent Events:** lists recent stream events by type.
- **Custom Widgets:** user-defined HTML, CSS, and JavaScript rendered as an OBS browser source.

Widget appearance options:

- Theme preset: `glass`, `minimal`, `cyber`, `retro`, `terminal`, or custom CSS.
- Accent color.
- Font family.
- Background opacity.
- Border radius.
- Width.
- Icon visibility where the widget supports it.

Most appearance fields can also be overridden from the OBS URL:

```text
http://localhost:30001/overlay/widgets/recent-events/?theme=terminal&accent=%2300ffd0&width=640&opacity=90
```

### Custom Widgets

Custom widgets are configured in the dashboard under **Widgets -> Custom Widgets**. Each custom widget has:

- Stable ID used by the OBS URL.
- Name.
- Enabled flag.
- Activation rules.
- HTML.
- CSS.
- JavaScript.
- Appearance settings.

OBS URL format:

```text
http://localhost:30001/overlay/widgets/custom/?id=custom-audio-alert
```

Custom widget JavaScript receives live activations through `raikiri:event`:

```js
window.addEventListener('raikiri:event', event => {
  const evt = event.detail;
  const user = evt.user || 'Viewer';
  document.getElementById('title').textContent = `${user} activated a reward`;
});
```

The full widget state is also available through `raikiri:state`:

```js
window.addEventListener('raikiri:state', event => {
  const state = event.detail;
  console.log(state.recentEvents);
});
```

Event fields available to custom widgets:

- `type`: `bits`, `channel_points`, `superchat`, `supersticker`, `membership`, `subscription`, `gift`, `raid`, `follow`.
- `platform`: `twitch` or `youtube`.
- `user`: username/display name from the platform.
- `amount`: bits, donation amount, gift count, or similar value.
- `currency`: currency label when available.
- `message`: chat message, superchat text, or channel points user input.
- `rewardName`: Twitch Channel Points reward title.
- `tier`: subscription tier when available.
- `viewers`: raid viewer count.

### Custom Widget Activations

Activation rules decide which live events trigger a custom widget.

Supported activation fields:

- **Event Type:** `any`, `bits`, `channel_points`, `superchat`, `supersticker`, `membership`, `subscription`, `gift`, `raid`, or `follow`.
- **Minimum Amount:** useful for bits, gifts, and paid support events.
- **Reward Name:** exact Twitch Channel Points reward title.

Examples:

- Bits alert: `eventType=bits`, `minAmount=25`.
- Channel Points alert: `eventType=channel_points`, `rewardName=Reunión G.A.T.O.`.
- Any support event: `eventType=any`.

The dashboard **Test** button for a custom widget generates a test event that matches that widget's activation rule.

## Local Media

Put local assets in:

```text
./data/media/
```

Reference them from the dashboard with `/media/...`.

Examples:

```text
/media/applause.mp3
/media/alert.gif
/media/boom.png
```

## Runtime Data

Raikiri stores runtime data under `--data-dir`.

Default if you follow the examples:

```text
./data/
```

Typical contents:

- `raikiri.db`
- `raikiri.db-wal`
- `raikiri.db-shm`
- `media/`
- `audio/`
- `logs/`

Back up this directory if you want to preserve configuration and tokens.

## Commands

Run the server:

```bash
raikiri serve --host 127.0.0.1 --port 30001 --data-dir ./data
```

Show version:

```bash
raikiri version
```

Initialize or migrate the database:

```bash
raikiri migrate --data-dir ./data
```

Bind to all network interfaces if another device must access it:

```bash
raikiri serve --host 0.0.0.0 --port 30001 --data-dir ./data
```

Only do this on a trusted local network.

## Building From Source

Raikiri uses `mise` to pin Go and build tools.

```bash
mise trust
mise install
mise run test
mise run build
```

The local development binary is written to:

```text
dist/raikiri
```

Run it:

```bash
./dist/raikiri serve --host 127.0.0.1 --port 30001 --data-dir ./data
```

## Release Builds

Build all release binaries and checksums:

```bash
mise run release-sha256
```

Artifacts are written to:

```text
dist/release/
```

Generated files:

- `raikiri-linux-amd64`
- `raikiri-linux-arm64`
- `raikiri-windows-amd64.exe`
- `raikiri-darwin-arm64`
- `raikiri-darwin-amd64`
- `SHA256SUMS`

Linux artifacts can be smoke-tested locally from a Linux machine. Windows and macOS artifacts are cross-compiled and should be tested on their target OS before publishing.

## Footprint

Measured locally on the native Go build:

- Linux amd64 binary: about `11 MB`.
- Full release set: about `55 MB`.
- Idle RSS: about `16 MB`.
- RSS after dashboard/config/chat/alert activity: about `23 MB`.

For comparison, the previous `2.1.2` Docker-based release used a much larger distribution footprint.

## Architecture

- Go HTTP server using the standard library.
- Embedded static dashboard and overlay assets.
- SQLite via a pure-Go driver.
- Native WebSocket endpoints under `/ws/*`.
- Dashboard config submit uses HTMX.
- OBS overlays consume native WebSockets, not Socket.IO.
- TTS is provided through Edge TTS behind a local queue.

## Important Notes

- YouTube web polling depends on YouTube's public page and internal continuation payload. If YouTube changes that shape, the adapter may need an update.
- Edge TTS is a free, unofficial integration. If it changes upstream, the TTS provider may need an update.
- Kick and TikTok are intentionally deferred to Phase 2.
- Keep `--data-dir` somewhere persistent. It contains your config, media, and auth tokens.

## Previous Version

If you need the pre-native implementation, use tag `2.1.2`.

```bash
git checkout 2.1.2
```

The native line is the recommended path going forward.
