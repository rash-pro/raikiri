import { z } from "zod";
import { configDb } from "./database.ts";

const ConfigSchema = z.object({
  twitchClientId: z.string().optional(),
  twitchChannel: z.string().optional(),
  youtubeChannelId: z.string().optional(),
  kickUsername: z.string().optional(),
  tiktokUsername: z.string().optional(),
  
  ttsEnabled: z.boolean().default(true),
  ttsVoice: z.string().default("es-MX-DaliaNeural"),
  ttsMinBits: z.number().default(100),
  ttsSubTier: z.number().default(3), // 1, 2, 3
  
  audioMode: z.enum(["websocket", "pulseaudio"]).default("websocket"),
  audioVolume: z.number().min(0).max(100).default(50),

  // Extended TTS Triggers
  ttsRewardEnabled: z.boolean().default(false),
  ttsRewardName: z.string().default(""),
  ttsCmdEnabled: z.boolean().default(false),
  ttsCmdPrefix: z.string().default("!voz"),
  ttsCmdMod: z.boolean().default(true),
  ttsCmdSub: z.boolean().default(false),
  ttsCmdVip: z.boolean().default(false),
  ttsCmdHost: z.boolean().default(true),
  
  // Chat Overlay Configuration
  chatTheme: z.enum(["dark", "light", "glassmorphism", "minimal", "ffvi", "cyberpurple"]).default("glassmorphism"),
  chatFontSize: z.number().min(8).max(40).default(15),
  chatHideAfter: z.number().min(0).max(120).default(30),
  chatAnimations: z.boolean().default(true),

  // Advanced Alerts Dictionary
  alertsConfig: z.any().default({}),
});

export type AppConfig = z.infer<typeof ConfigSchema>;

// Default configuration fallback
const defaultConfig: AppConfig = {
  ttsEnabled: true,
  ttsVoice: "es-MX-DaliaNeural",
  ttsMinBits: 100,
  ttsSubTier: 3,
  audioMode: "websocket",
  audioVolume: 50,
  ttsRewardEnabled: false,
  ttsRewardName: "",
  ttsCmdEnabled: false,
  ttsCmdPrefix: "!voz",
  ttsCmdMod: true,
  ttsCmdSub: false,
  ttsCmdVip: false,
  ttsCmdHost: true,
  chatTheme: "glassmorphism",
  chatFontSize: 15,
  chatHideAfter: 30,
  chatAnimations: true,
  alertsConfig: {
      follow: { enabled: true, theme: "cyberpurple", voice: "", gifUrl: "", audioUrl: "", messageTemplate: "¡{user} ha comenzado a seguirte!" },
      subscription: { enabled: true, theme: "cyberpurple", voice: "", gifUrl: "", audioUrl: "", messageTemplate: "¡{user} se ha suscrito (Tier {tier})! {message}" },
      bits: { enabled: true, theme: "cyberpurple", voice: "", gifUrl: "", audioUrl: "", messageTemplate: "{user} ha donado {amount} bits: {message}" },
      raid: { enabled: true, theme: "cyberpurple", voice: "", gifUrl: "", audioUrl: "", messageTemplate: "¡Alerta de Raid! {user} trae {amount} espectadores." },
      superchat: { enabled: true, theme: "cyberpurple", voice: "", gifUrl: "", audioUrl: "", messageTemplate: "¡{user} donó {amount} súper chat! {message}" },
      gift: { enabled: true, theme: "cyberpurple", voice: "", gifUrl: "", audioUrl: "", messageTemplate: "¡{user} ha regalado {amount} suscripciones!" },
      channel_points: { enabled: true, theme: "cyberpurple", voice: "", gifUrl: "", audioUrl: "", messageTemplate: "{user} dice: {message}" }
  }
};

export class ConfigManager {
  private currentConfig: AppConfig;

  constructor() {
    this.currentConfig = { ...defaultConfig };
    this.load();
  }

  load() {
    const loadedConfig: any = {};
    for (const key of Object.keys(ConfigSchema.shape)) {
      const val = configDb.get(key);
      if (val !== null) {
        try {
          // Attempt to parse JSON if it's a boolean or number
          loadedConfig[key] = JSON.parse(val);
        } catch (_) {
          loadedConfig[key] = val;
        }
      }
    }
    
    const parsed = ConfigSchema.safeParse(loadedConfig);
    if (parsed.success) {
      this.currentConfig = { ...defaultConfig, ...parsed.data };
    } else {
      console.error("Config validation failed on load:", parsed.error);
    }
  }

  get<K extends keyof AppConfig>(key: K): AppConfig[K] {
    return this.currentConfig[key];
  }

  set<K extends keyof AppConfig>(key: K, value: AppConfig[K]) {
    this.currentConfig[key] = value;
    configDb.set(key as string, typeof value === "string" ? value : JSON.stringify(value));
  }
}

export const config = new ConfigManager();
