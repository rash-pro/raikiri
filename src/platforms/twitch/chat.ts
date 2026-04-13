import tmi from 'tmi.js';
import { PlatformAdapter } from '../base.ts';
import { sanitizeMessage } from '../../utils/sanitizer.ts';

export class TwitchChatAdapter extends PlatformAdapter {
  private client: tmi.Client;
  private channels: string[];

  constructor(channels: string[], identity?: { username: string; token: string }) {
    super('twitch');
    this.channels = channels;
    
    const options: tmi.Options = {
      connection: {
        secure: true,
        reconnect: true,
      },
      channels: this.channels,
    };

    if (identity && identity.token) {
      options.identity = {
        username: identity.username,
        password: `oauth:${identity.token}`
      };
    }

    this.client = new tmi.Client(options);

    this.setupListeners();
  }

  private setupListeners() {
    this.client.on('message', (channel, tags, message, self) => {
      if (self) return;

      const user = tags['display-name'] || tags['username'] || 'unknown';
      
      const chatMessage = {
        id: tags['id'] || Date.now().toString(),
        user: user,
        displayName: user,
        content: message,
        htmlContent: this.parseMessage(message, tags.emotes),
        color: tags['color'],
        badges: this.parseBadges(tags.badges),
        timestamp: new Date().toISOString(),
      };

      this.emitChat({ msg: chatMessage, user: user, content: message });
    });

    this.client.on('connected', (address, port) => {
      this.logger.info(`Connected to ${address}:${port}`);
    });

    this.client.on('disconnected', (reason) => {
      this.logger.warn(`Disconnected: ${reason}`);
    });
  }

  private parseMessage(message: string, emotes: any): string {
    if (!emotes) return sanitizeMessage(message);

    const replacements: { start: number, end: number, id: string }[] = [];
    Object.keys(emotes).forEach(id => {
      emotes[id].forEach((range: string) => {
        const [start, end] = range.split('-').map(Number);
        replacements.push({ start, end, id });
      });
    });

    replacements.sort((a, b) => b.start - a.start);

    let result = "";
    let lastIndex = message.length;

    replacements.forEach(rep => {
      const tail = message.substring(rep.end + 1, lastIndex);
      result = sanitizeMessage(tail) + result;

      const emoteUrl = `https://static-cdn.jtvnw.net/emoticons/v2/${rep.id}/default/dark/1.0`;
      const imgTag = `<img src="${emoteUrl}" class="emote" alt="emote">`;
      result = imgTag + result;

      lastIndex = rep.start;
    });

    const head = message.substring(0, lastIndex);
    result = sanitizeMessage(head) + result;

    return result;
  }

  private parseBadges(badges: any): any[] {
    if (!badges) return [];
    const normalized = [];
    if (badges.moderator) normalized.push({ type: 'moderator', url: '' });
    if (badges.broadcaster) normalized.push({ type: 'owner', url: '' });
    if (badges.subscriber) normalized.push({ type: 'subscriber', url: '' });
    if (badges.vip) normalized.push({ type: 'vip', url: '' });
    // In v2.0, we can refine how we handle badge URLs using Twitch API, but for now we keep the same logic structure
    return normalized;
  }

  async connect(): Promise<void> {
    try {
      if (this.channels.length > 0) {
        await this.client.connect();
      } else {
        this.logger.warn("No Twitch channels configured to connect.");
      }
    } catch (err) {
      this.logger.error('Failed to connect to Twitch', err);
    }
  }

  async disconnect(): Promise<void> {
    try {
      await this.client.disconnect();
    } catch (err) {
      this.logger.error('Error disconnecting from Twitch', err);
    }
  }
}
