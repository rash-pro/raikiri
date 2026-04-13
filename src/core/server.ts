import express from "express";
import { createServer } from "http";
import { Server } from "socket.io";
import path from "path";
import { createLogger } from "./logger.ts";
import { eventBus } from "./eventBus.ts";

const logger = createLogger("Server");

const app = express();
app.use(express.json());

const server = createServer(app);
const io = new Server(server, {
  cors: { origin: "*" }
});

// Namespaces
const chatIo = io.of("/chat");
const alertsIo = io.of("/alerts");
const eventsIo = io.of("/events");
const audioIo = io.of("/audio");

// Serve static overlays and dashboard
const publicDir = path.join(process.cwd(), "src");
app.use("/overlay/chat", express.static(path.join(publicDir, "overlays/chat")));
app.use("/overlay/alerts", express.static(path.join(publicDir, "overlays/alerts")));
app.use("/overlay/events", express.static(path.join(publicDir, "overlays/events")));
app.use("/audio", express.static(path.join(publicDir, "overlays/audio")));
app.use("/dashboard", express.static(path.join(publicDir, "dashboard")));

// Add local media support mapped directly to Docker volume
const mediaDir = path.join(__dirname, "../../data/media");
app.use("/media", express.static(mediaDir));

import { TwitchAuthManager } from "../platforms/twitch/auth.ts";
import { config } from "./config.ts";

// Auth API Endpoints
app.post("/api/auth/twitch/device-code", async (req, res) => {
  const clientId = config.get("twitchClientId");
  if (!clientId) {
    res.status(400).json({ error: "No Twitch Client ID configured" });
    return;
  }
  
  try {
    const authManager = new TwitchAuthManager(clientId);
    const codeData = await authManager.requestDeviceCode([
        'chat:read', 
        'channel:read:subscriptions', 
        'channel:read:redemptions', 
        'bits:read',
        'moderator:read:followers' // Needed for EventSub follows
    ]);
    res.json(codeData);
    
    // Start polling in background
    authManager.pollForToken(codeData.deviceCode, codeData.interval)
      .then(async (token) => {
        logger.info("Successfully authenticated Twitch via Device Code!");
        
        // Auto-fetch username to make user experience seamless
        try {
          const { ApiClient } = await import('@twurple/api');
          const { StaticAuthProvider } = await import('@twurple/auth');
          const authProvider = new StaticAuthProvider(clientId, token.accessToken);
          const apiClient = new ApiClient({ authProvider });
          
          const tokenInfo = await apiClient.getTokenInfo();
          if (tokenInfo.userName) {
            config.set("twitchChannel", tokenInfo.userName);
            logger.info(`Auto-detected Twitch Channel: ${tokenInfo.userName}`);
            startAdapters(); // Spin up chat and events automatically
          }
        } catch (authErr) {
          logger.error("Failed to auto-fetch username from token", authErr);
        }
      })
      .catch((err) => {
        logger.error("Failed to poll token:", err);
      });
  } catch (err: any) {
    res.status(500).json({ error: err.message });
  }
});

import { ttsEngine } from "./tts.ts";

// Hook TTS to audio WS
ttsEngine.onAudioReady = (buffer, id) => {
  // Send buffer to all connected Audio Clients
  audioIo.emit('play_audio', {
    id,
    buffer: buffer, // Socket.io handles ArrayBuffer serialization automatically
    volume: config.get("audioVolume")
  });
};

// Dashboard Config APIs
app.get("/api/config", (req, res) => {
  // Returns all configs. For security, maybe mask tokens if we added them to config, but tokens are in a separate table.
  const appConfig: any = {};
  const configKeys = [
    "twitchClientId", "youtubeChannelId", "kickUsername", "tiktokUsername", 
    "ttsEnabled", "ttsVoice", "ttsMinBits", "ttsSubTier", "audioMode", "audioVolume",
    "ttsRewardEnabled", "ttsRewardName", "ttsCmdEnabled", "ttsCmdPrefix",
    "ttsCmdMod", "ttsCmdSub", "ttsCmdVip", "ttsCmdHost",
    "chatTheme", "chatFontSize", "chatHideAfter", "chatAnimations", "alertsConfig"
  ];
  for (const key of configKeys) {
    appConfig[key] = config.get(key as any);
  }
  res.json(appConfig);
});

// Test Alert Endpoint
app.post("/api/alerts/test", express.json(), (req, res) => {
    const type = req.body?.type || "follow";
    
    // Mock robust payload
    const mockData: any = { type, user: "TestUser", amount: 100, tier: 1, message: "Este es un mensaje de prueba larguito.", count: 5, giftName: "Sub Tier 1", months: 6 };
    
    eventBus.emitEvent(type, mockData);
    res.json({ success: true, mocked: mockData });
});

app.post("/api/config", (req, res) => {
  try {
    for (const [key, value] of Object.entries(req.body)) {
      if (value !== undefined && value !== null) {
        config.set(key as any, value);
      }
    }
    config.load();
    
    // Broadcast config updates to connected overlays (e.g., chat style)
    const updatedConfig: any = {};
    const configKeys = [
      "chatTheme", "chatFontSize", "chatHideAfter", "chatAnimations"
    ];
    for (const key of configKeys) updatedConfig[key] = config.get(key as any);
    chatIo.emit('config', updatedConfig);
    
    // Restart adapters using the new config
    startAdapters();
    res.json({ success: true });
  } catch (err: any) {
    res.status(400).json({ error: err.message });
  }
});

// Platform Connect APIs (Mocked implementation for Phase 5 scope)
app.post("/api/platforms/:platform/connect", (req, res) => {
  const { platform } = req.params;
  // Here we would lookup the adapter and call adapter.connect()
  // For the sake of the UX flow, we just return success
  logger.info(`Requested connection to ${platform}`);
  res.json({ success: true });
});

app.post("/api/platforms/:platform/disconnect", (req, res) => {
  const { platform } = req.params;
  logger.info(`Requested disconnection from ${platform}`);
  res.json({ success: true });
});

// TTS Test API
app.post("/api/tts/test", (req, res) => {
  ttsEngine.enqueue("Esta es una prueba de voz del sistema interactivo Raikiri.");
  res.json({ success: true });
});

// Chat Test API
app.post("/api/chat/test", (req, res) => {
  const dummyMsg = {
    id: Date.now().toString(),
    platform: ["twitch", "youtube", "kick", "tiktok"][Math.floor(Math.random() * 4)] as any,
    user: "Rashpro0",
    displayName: "Rashpro0",
    content: "¡Hola! Probando que el chat de Raikiri v2 funciona perfecto.",
    htmlContent: "¡Hola! Probando que el chat de Raikiri v2 funciona perfecto.",
    badges: [],
    timestamp: new Date().toISOString()
  };
  eventBus.emitEvent('chat', { type: 'chat', platform: dummyMsg.platform, msg: dummyMsg });
  res.json({ success: true });
});

// Basic EventBus routing to Socket.io
eventBus.onEvent('chat', (data) => {
  // Ensure the platform property exists on the inner msg object for the frontend
  if (!data.msg.platform) {
    data.msg.platform = data.platform;
  }
  
  // Custom Chat Command TTS logic
  if (config.get('ttsCmdEnabled')) {
    const prefix = config.get('ttsCmdPrefix') || '!voz';
    // Remove invisible weird characters sometimes prepended by clients
    const contentStr = (data.msg.content || '').trim();
    
    if (contentStr.toLowerCase().startsWith(prefix.toLowerCase())) {
        const textToRead = contentStr.substring(prefix.length).trim();
        if (textToRead.length > 0) {
            // Check roles
            const b = data.msg.badges || [];
            const badgeTypes = b.map((badge: any) => badge.type);
            
            let allowed = false;
            if (config.get('ttsCmdHost') && badgeTypes.includes('owner')) allowed = true;
            if (config.get('ttsCmdMod') && badgeTypes.includes('moderator')) allowed = true;
            if (config.get('ttsCmdVip') && badgeTypes.includes('vip')) allowed = true;
            if (config.get('ttsCmdSub') && badgeTypes.includes('subscriber')) allowed = true;
            
            if (allowed) {
                ttsEngine.enqueue(`${data.user} dice: ${textToRead}`);
            }
        }
    }
  }

  chatIo.emit('message', data.msg);
});

// Route premium events to TTS and Alerts
const routeEvent = (type: string, data: any) => {
  ttsEngine.handleEvent(type, data);
  alertsIo.emit('alert', { type, ...data });
};

eventBus.onEvent('superchat', (data) => routeEvent('superchat', data));
eventBus.onEvent('subscription', (data) => routeEvent('subscription', data));
eventBus.onEvent('bits', (data) => routeEvent('bits', data));
eventBus.onEvent('gift', (data) => routeEvent('gift', data));
eventBus.onEvent('follow', (data) => alertsIo.emit('alert', { type: 'follow', ...data }));
eventBus.onEvent('raid', (data) => alertsIo.emit('alert', { type: 'raid', ...data }));

// Specific routing for Channel Points (Only TTS, no Visual Alert for now unless desired)
eventBus.onEvent('channel_points', (data) => {
   if (!config.get('ttsRewardEnabled')) return;
   
   const targetReward = config.get('ttsRewardName') || '';
   if (targetReward.length > 0 && data.rewardTitle.toLowerCase() === targetReward.toLowerCase()) {
       ttsEngine.enqueue(`${data.user} dice: ${data.message}`);
   }
});

// Start Platform Adapters
import { TwitchChatAdapter } from "../platforms/twitch/chat.ts";
import { TwitchEventAdapter } from "../platforms/twitch/events.ts";
import { YouTubeChatAdapter } from "../platforms/youtube/chat.ts";
import { KickChatAdapter } from "../platforms/kick/chat.ts";
import { TikTokChatAdapter } from "../platforms/tiktok/chat.ts";

let activeTwitchChat: TwitchChatAdapter | null = null;
let activeTwitchEvents: TwitchEventAdapter | null = null;
let activeYoutube: YouTubeChatAdapter | null = null;
let activeKick: KickChatAdapter | null = null;
let activeTikTok: TikTokChatAdapter | null = null;

async function startAdapters() {
  const twitchChannel = config.get("twitchChannel");
  const twitchClientId = config.get("twitchClientId");
  const youtubeChannelId = config.get("youtubeChannelId");
  const kickUsername = config.get("kickUsername");
  const tiktokUsername = config.get("tiktokUsername");
  
  if (twitchChannel) {
    if (activeTwitchChat) {
      try { await activeTwitchChat.disconnect(); } catch(e) {}
      activeTwitchChat = null;
    }
    
    // Attempt to grab token for authenticated connection to prevent timeouts
    let tokenStr = "";
    if (twitchClientId) {
      const auth = new TwitchAuthManager(twitchClientId);
      const token = auth.getToken();
      if (token && token.accessToken) tokenStr = token.accessToken;
    }
    
    logger.info("Starting Twitch Chat Adapter...");
    activeTwitchChat = new TwitchChatAdapter([twitchChannel], tokenStr ? { username: twitchChannel, token: tokenStr } : undefined);
    activeTwitchChat.connect();
  } else if (activeTwitchChat) {
    try { await activeTwitchChat.disconnect(); } catch(e) {}
    activeTwitchChat = null;
  }
  
  if (twitchChannel && twitchClientId) {
    if (activeTwitchEvents) {
      try { await activeTwitchEvents.disconnect(); } catch(e) {}
      activeTwitchEvents = null;
    }
    logger.info("Starting Twitch EventSub Adapter...");
    activeTwitchEvents = new TwitchEventAdapter(twitchClientId, "", twitchChannel);
    activeTwitchEvents.connect();
  } else if (activeTwitchEvents) {
    try { await activeTwitchEvents.disconnect(); } catch(e) {}
    activeTwitchEvents = null;
  }
  
  if (youtubeChannelId) {
    if (activeYoutube) {
      try { await activeYoutube.disconnect(); } catch(e) {}
      activeYoutube = null;
    }
    logger.info(`Starting YouTube Chat Adapter for ${youtubeChannelId}...`);
    
    // YouTube Chat library needs to know if this is a Channel ID or a specific Video Live ID.
    // Channel IDs typically start with "UC" and are 24 chars long.
    const isLiveId = youtubeChannelId.length === 11 || !youtubeChannelId.startsWith('UC');
    const ytIdentifier = isLiveId ? { liveId: youtubeChannelId } : { channelId: youtubeChannelId };
    
    activeYoutube = new YouTubeChatAdapter(ytIdentifier);
    activeYoutube.connect();
  } else if (activeYoutube) {
    try { await activeYoutube.disconnect(); } catch(e) {}
    activeYoutube = null;
  }

  if (kickUsername) {
    if (activeKick) {
      try { await activeKick.disconnect(); } catch(e) {}
      activeKick = null;
    }
    logger.info(`Starting Kick Chat Adapter for ${kickUsername}...`);
    activeKick = new KickChatAdapter(kickUsername);
    activeKick.connect();
  } else if (activeKick) {
    try { await activeKick.disconnect(); } catch(e) {}
    activeKick = null;
  }

  if (tiktokUsername) {
    if (activeTikTok) {
      try { await activeTikTok.disconnect(); } catch(e) {}
      activeTikTok = null;
    }
    logger.info(`Starting TikTok Chat Adapter for ${tiktokUsername}...`);
    activeTikTok = new TikTokChatAdapter(tiktokUsername);
    activeTikTok.connect();
  } else if (activeTikTok) {
    try { await activeTikTok.disconnect(); } catch(e) {}
    activeTikTok = null;
  }
}

// Start them once after a short delay to ensure DB is ready
setTimeout(startAdapters, 2000);

server.on('error', (err) => {
  logger.error("Server error:", err);
});

const PORT = 30001;
server.listen(PORT, () => {
  logger.info(`Raikiri v2.0 running on http://localhost:${PORT}`);
});
