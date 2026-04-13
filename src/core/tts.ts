import { EdgeTTS } from 'edge-tts-universal';
import path from 'path';
import fs from 'fs';
import { createLogger } from './logger.ts';
import { config } from './config.ts';

const logger = createLogger('TTS');
const audioDir = path.join(process.cwd(), 'data', 'audio');

if (!fs.existsSync(audioDir)) {
  fs.mkdirSync(audioDir, { recursive: true });
}

export class TTSEngine {
  private queue: { text: string, id: string }[] = [];
  private isProcessing = false;
  // Callback when audio is ready to be played (emits the buffer/url)
  public onAudioReady?: (buffer: Buffer, id: string) => void;

  constructor() { }

  public enqueue(text: string, voiceOverride?: string) {
    if (!config.get("ttsEnabled")) return;
    
    // Simple filter: limit length
    const cleanText = text.substring(0, 150).replace(/[<>]/g, '');
    if (!cleanText.trim()) return;

    this.queue.push({ text: cleanText, id: Date.now().toString(), voice: voiceOverride });
    logger.debug(`Enqueued TTS: "${cleanText}"`);
    this.processQueue();
  }

  private async processQueue() {
    if (this.isProcessing || this.queue.length === 0) return;
    this.isProcessing = true;

    const item = this.queue.shift();
    if (!item) {
        this.isProcessing = false;
        return;
    }
    const targetVoice = item.voice || config.get("ttsVoice");

    try {
      logger.info(`Synthesizing TTS via cloud: ${item.text}`);
      
      // edge-tts-universal API
      const tts = new EdgeTTS(item.text, targetVoice);
      const result = await tts.synthesize();
      
      const arrayBuffer = await result.audio.arrayBuffer();
      const buffer = Buffer.from(arrayBuffer);
      
      const mode = config.get("audioMode");
      if (mode === 'pulseaudio') {
         // Direct playback via aplay (Linux only)
         // Assuming stream is 24kHz 16-bit mono PCM (EdgeTTS standard fallback)
         // Or just pipe it to an mp3 player like mpg123 if it returns mp3
         Bun.spawn(["aplay", "-q"], {
           stdin: buffer
         });
         logger.debug(`Played TTS via local audio: ${item.id}`);
      } else {
        if (this.onAudioReady) {
          this.onAudioReady(buffer, item.id);
        } else {
          logger.warn("TTS generated but no audio handler is attached (WebSocket mode).");
        }
      }
    } catch (err) {
      logger.error(`TTS Generation Failed for "${item.text}"`, err);
    } finally {
      this.isProcessing = false;
      // Allow a small delay between TTS messages for natural flow
      setTimeout(() => this.processQueue(), 500);
    }
  }

  // Pre-configured filter wrapper: Only triggers for "premium" events
  public handleEvent(eventType: string, data: any) {
    if (!config.get("ttsEnabled")) return;
    
    const alertsConf = config.get("alertsConfig") || {};
    const evtConf = alertsConf[eventType];
    
    if (evtConf && !evtConf.enabled) return;

    let msg = evtConf && evtConf.messageTemplate ? evtConf.messageTemplate : "";
    let voice = evtConf && evtConf.voice ? evtConf.voice : undefined;

    // Replace variables
    if (msg) {
        msg = msg.replace(/{user}/g, data.user || 'Alguien')
                 .replace(/{amount}/g, data.amount || data.count || data.viewers || '')
                 .replace(/{tier}/g, data.tier || '')
                 .replace(/{message}/g, data.message || '');
                 
        this.enqueue(msg, voice);
    } else {
        // Fallback for types not strictly defined in config yet
        if (eventType === 'channel_points' && config.get('ttsRewardEnabled')) {
             this.enqueue(`${data.user} dice: ${data.message}`);
        }
    }
  }
}

export const ttsEngine = new TTSEngine();
