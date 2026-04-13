import Pusher from 'pusher-js';
import { PlatformAdapter } from '../base.ts';
import { sanitizeMessage } from '../../utils/sanitizer.ts';

export class KickChatAdapter extends PlatformAdapter {
  private channelName: string;
  private pusher: Pusher | null = null;
  private chatroomId: string | null = null;
  private channelId: string | null = null;

  constructor(channelName: string) {
    super('kick');
    this.channelName = channelName;
  }

  async connect(): Promise<void> {
    try {
      this.logger.info(`Fetching channel info for ${this.channelName}...`);
      
      const userAgent = 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/121.0.0.0 Safari/537.36';
      const url = `https://kick.com/api/v2/channels/${this.channelName}`;

      // Using Bun's native fetch. It often bypasses WAF better than Node's axios.
      const response = await fetch(url, {
        headers: {
          'User-Agent': userAgent,
          'Accept': 'application/json'
        }
      });

      if (!response.ok) {
        throw new Error(`HTTP error! status: ${response.status}`);
      }

      const data = await response.json();

      if (data && data.chatroom) {
        this.chatroomId = data.chatroom.id;
        this.channelId = data.id;
        this.setupPusher();
      } else {
        throw new Error(data.message || 'Channel details not found');
      }
    } catch (error: any) {
      this.logger.error('Failed to connect to Kick:', error.message);
    }
  }

  private setupPusher() {
    this.pusher = new Pusher('32cbd69e4b950bf97679', {
      cluster: 'us2',
      wsHost: 'ws-us2.pusher.com',
      wsPort: 443,
      wssPort: 443,
      forceTLS: true,
      enabledTransports: ['ws', 'wss']
    });

    const channel = this.pusher.subscribe(`chatrooms.${this.chatroomId}.v2`);

    channel.bind('App\\Events\\ChatMessageEvent', (data: any) => {
      this.handleMessage(data);
    });

    this.pusher.connection.bind('connected', () => {
      this.logger.info(`Connected to Kick chat: ${this.channelName}`);
    });

    this.pusher.connection.bind('error', (err: any) => {
      this.logger.error('Pusher error:', err);
    });
  }

  private handleMessage(data: any) {
    const sender = data.sender;
    const user = sender.username;

    const content = this.parseEmotes(data.content);

    const chatMessage = {
      id: data.id,
      platform: this.platform,
      user: user,
      displayName: user,
      color: sender.identity.color || '#53fc18',
      content: sanitizeMessage(data.content),
      htmlContent: content, // Emote parsing result
      badges: this.normalizeBadges(sender.identity.badges),
      timestamp: new Date().toISOString()
    };

    this.emitChat({ msg: chatMessage, user: user, content: data.content });
  }

  private normalizeBadges(badges: any[]): any[] {
    const normalized: any[] = [];
    if (!badges) return normalized;

    badges.forEach(b => {
      const type = b.type.toLowerCase();
      if (type === 'broadcaster' || type === 'owner') normalized.push({ type: 'owner', url: '' });
      else if (type === 'moderator') normalized.push({ type: 'moderator', url: '' });
      else if (type === 'vip') normalized.push({ type: 'vip', url: '' });
      else if (type === 'subscriber') normalized.push({ type: 'subscriber', url: '' });
    });

    return normalized;
  }

  private parseEmotes(content: string): string {
    const emoteRegex = /\[emote:(\d+):([\w\d]+)\]/g;
    let safeContent = sanitizeMessage(content);
    
    // We sanitize first, then replace the specific emote strings with HTML
    safeContent = safeContent.replace(/\[emote:(\d+):([\w\d]+)\]/g, (match, id, name) => {
      return `<img src="https://files.kick.com/emotes/${id}/fullsize" alt="${name}" title="${name}" class="emote">`;
    });
    return safeContent;
  }

  async disconnect(): Promise<void> {
    if (this.pusher) {
      this.pusher.disconnect();
      this.pusher = null;
    }
  }
}
