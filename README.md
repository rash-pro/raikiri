# Raikiri Native

Raikiri is a self-hosted streaming interaction hub for OBS. It aggregates live chat and stream events into local browser overlays, alert scenes, and browser-based TTS audio.

This is the native Go rewrite. It ships as a small standalone binary. If you need the previous Bun/Docker implementation, it remains available at tag `2.1.2`.

## Version Line

- **Current line:** Raikiri Native, built in Go.
- **Previous line:** the Bun/Docker implementation remains available at tag `2.1.2`.
- **Native platforms:** Twitch, YouTube Live, and TikTok LIVE.
- **Planned platform:** Kick remains disabled until its adapter is implemented.

## What You Get

- Native executable for Linux, Windows, and macOS, plus a Windows installer that runs Raikiri from the system tray.
- Embedded dashboard and OBS overlay assets.
- Local SQLite config/database under your chosen data directory.
- Twitch chat and EventSub alerts, YouTube Live chat via web-first polling, and TikTok LIVE chat/events.
- Edge TTS cloud voices streamed to `/audio`.
- Local media support via `/media/*`.
- OBS browser sources for chat, alerts, widgets, and audio.
- Configurable widgets for support goals, recent events, and user-defined custom overlays.
- Steam achievement toasts (including FFNx-based Final Fantasy releases) and a Final Fantasy roulette widget.
- A stream-info dock that sets title, game, and tags on Twitch and YouTube at once, with title and tag ideas from a local AI agent CLI (Claude Code or Codex).

## Downloading a Release

Download the binary for your OS from the release artifacts:

- **Windows (recommended): `raikiri-windows-amd64-setup.exe`** — installer, see below.
- Windows x64 portable/CLI: `raikiri-windows-amd64.exe`
- Linux x64: `raikiri-linux-amd64`
- Linux ARM64: `raikiri-linux-arm64`
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

### Windows (installer)

Run `raikiri-windows-amd64-setup.exe`. It installs for your user only (no administrator prompt) into `%LOCALAPPDATA%\Programs\Raikiri`, adds a Start menu entry, and optionally a desktop shortcut and **Start with Windows**.

Raikiri then runs from a tray icon next to the clock (**Open dashboard**, **Open data folder**, **Quit**) instead of a console window. The dashboard opens by itself on the first run; launching Raikiri again while it is running just opens the dashboard. Settings, tokens, and logs live in `%APPDATA%\Raikiri` (`raikiri.log` there is the place to look when something fails).

The installer is not code-signed yet, so Windows SmartScreen may say "Windows protected your PC": choose **More info → Run anyway**.

Uninstall from **Settings → Apps**. It asks before deleting `%APPDATA%\Raikiri`, so your configuration survives reinstalls by default.

### Windows PowerShell (portable)

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
3. Configure Twitch, YouTube, and/or TikTok.
4. Click **Save Configuration**.
5. Add the OBS browser sources listed below.

### Twitch

For Twitch chat only, set **Channel Username** in the dashboard and save.

For Twitch EventSub alerts and the stream-info dock, create a Twitch developer app, copy its **Client ID** and a **Client Secret**, save them in the dashboard, then click **Authenticate Device**. Open the activation link and enter the displayed code. Raikiri stores the token locally in SQLite under `--data-dir` and renews it on its own; without the client secret the token can't be renewed and you'd have to authenticate before every stream.

The dashboard's Twitch card links a step-by-step visual guide (`/dashboard/guides/twitch.html`).

### YouTube

Set **Channel ID**, **Video ID**, or handle-like live target in the YouTube field and save.

The native adapter is web-first: it follows YouTube's public live page and Innertube continuation flow, similar to the previous JavaScript implementation. Reading chat does not require a YouTube API key.

Changing the stream title and tags from the stream-info dock needs a Google OAuth **Desktop app** client: paste its client ID and secret in the YouTube card, save, and click **Connect YouTube**. The card links a visual guide (`/dashboard/guides/youtube.html`). While the Google app is in *Testing*, Google expires the sign-in every 7 days; reconnect when the dock shows YouTube disconnected.

### TikTok LIVE

Set the creator's TikTok `@username` and save while that creator is live. Raikiri reads public LIVE chat plus gifts, follows, likes, and shares without requiring a TikTok login. Gift streaks produce one event after the streak finishes, with the final gift count and diamond value.

Likes are delivered live to custom widgets but are intentionally not written to SQLite because busy streams can produce many like batches per second.

Raikiri owns this integration in its internal `tiktoklive` module: room discovery, WebSocket framing, protobuf decoding, reconnection, gift streaks, and normalized events all ship in the Raikiri binary. It does not call an external signing service or require the PirateTok module at runtime or build time. The initial protocol implementation was derived from PirateTok/live-go under its 0BSD license; see `internal/tiktoklive/NOTICE` for attribution.

TikTok does not provide a public official API for reading these LIVE events, so the integration may require updates if TikTok changes its internal Webcast protocol.

### Kick

Kick is not implemented in this native version yet. Its dashboard input remains disabled.

## OBS Sources

Add these as OBS Browser Sources:

- Chat overlay: `http://localhost:30001/overlay/chat/`
- Alerts overlay: `http://localhost:30001/overlay/alerts/`
- Support goal widget: `http://localhost:30001/overlay/widgets/support-goal/`
- Recent events widget: `http://localhost:30001/overlay/widgets/recent-events/`
- Custom widget: `http://localhost:30001/overlay/widgets/custom/?id=YOUR_WIDGET_ID`
- Steam achievements: `http://localhost:30001/overlay/widgets/achievements/`
- Final Fantasy roulette: `http://localhost:30001/overlay/widgets/roulette/` (spin from the dashboard or `POST /api/widgets/roulette/spin`)
- Audio output: `http://localhost:30001/audio/`

Suggested sizes:

- Chat: `400x800`, or your preferred chat column size.
- Alerts: `1920x1080`.
- Widgets: match the widget width configured in the dashboard, or use a transparent `1920x1080` source.
- Audio: any size; enable **Control audio via OBS** if desired.

If OBS or the browser blocks autoplay on `/audio`, right-click the source, choose **Interact**, and press **Enable Audio** once.

## Stream Info Dock

`http://localhost:30001/dock/stream-info/` sets the title, game, and tags on Twitch and YouTube at once. Use it as an OBS Custom Browser Dock or keep it open in a browser tab.

- Twitch gets title, category, and tags; YouTube gets title and tags (plus the Gaming category), because the YouTube API can't set the specific game. If no YouTube broadcast exists yet, the title is applied within 30 seconds of going live.
- **Sugerir** asks an AI agent CLI installed on the same machine for title and tag ideas, based on your recent titles, past Twitch broadcasts, and Steam achievement progress for the current game. Pick the CLI (Claude Code or Codex) and model in the dashboard under **Assistant**. It uses the CLI's own login; no API key is stored in Raikiri.

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

- `type`: `bits`, `channel_points`, `superchat`, `supersticker`, `membership`, `subscription`, `gift`, `raid`, `follow`, `like`, or `share`.
- `platform`: `twitch`, `youtube`, or `tiktok`.
- `user`: username/display name from the platform.
- `amount`: bits, donation amount, TikTok diamond value, or similar value.
- `count`: gift or like count when the platform supplies one.
- `giftName`: TikTok gift name when available.
- `currency`: currency label when available.
- `message`: chat message, superchat text, or channel points user input.
- `rewardName`: Twitch Channel Points reward title.
- `tier`: subscription tier when available.
- `viewers`: raid viewer count.

### Custom Widget Activations

Activation rules decide which live events trigger a custom widget.

Supported activation fields:

- **Event Type:** `any`, `bits`, `channel_points`, `superchat`, `supersticker`, `membership`, `subscription`, `gift`, `raid`, `follow`, `like`, or `share`.
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

Run from a Windows tray icon (what the installer's shortcuts do; logs go to `<data-dir>/raikiri.log`):

```powershell
raikiri.exe --tray --data-dir "$env:APPDATA\Raikiri"
```

Flags without a command mean `serve`.

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

The TikTok protocol suite uses synthetic Webcast frames by default. To exercise room discovery and a real public stream, run the opt-in integration test with a creator who is currently live:

```bash
RAIKIRI_TIKTOK_LIVE_USER=creator go test -tags=integration ./internal/tiktoklive -run TestLiveConnectionAndTraffic -v
```

## Release Builds

Build all release binaries, the Windows installer, and checksums (the installer needs `makensis`, or Docker to run it from Debian's `nsis` package):

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
- `raikiri-windows-amd64-setup.exe`
- `raikiri-darwin-arm64`
- `raikiri-darwin-amd64`
- `THIRD_PARTY_NOTICES`
- `SHA256SUMS`

Linux artifacts can be smoke-tested locally from a Linux machine. The Windows installer can be smoke-tested with Wine (`wine raikiri-windows-amd64-setup.exe /S`), but Windows and macOS artifacts should still be tested on their target OS.

## Footprint

Measured locally on the native Go build:

- Linux amd64 binary: about `15 MB`.
- Full release set: about `74 MB`.
- Idle RSS: about `16 MB`.
- RSS after dashboard/config/chat/alert activity: about `23 MB`.

For comparison, the previous `2.1.2` Docker-based release used a much larger distribution footprint.

## Architecture

- Go HTTP server using the standard library.
- Embedded static dashboard and overlay assets.
- SQLite via a pure-Go driver.
- Native WebSocket endpoints under `/ws/*`.
- TikTok Webcast transport and protocol details are private to `internal/tiktoklive`; the application consumes only Raikiri's normalized event interface.
- Dashboard config submit uses HTMX.
- OBS overlays consume native WebSockets, not Socket.IO.
- TTS is provided through Edge TTS behind a local queue.

## Important Notes

- YouTube web polling depends on YouTube's public page and internal continuation payload. If YouTube changes that shape, the adapter may need an update.
- TikTok LIVE support depends on TikTok's unofficial internal Webcast protocol. It reads public streams only and may need an update when TikTok changes that protocol.
- Edge TTS is a free, unofficial integration. If it changes upstream, the TTS provider may need an update.
- Kick remains deferred.
- Keep `--data-dir` somewhere persistent. It contains your config, media, and auth tokens.

## Previous Version

If you need the pre-native implementation, use tag `2.1.2`.

```bash
git checkout 2.1.2
```

The native line is the recommended path going forward.
