import { LiveChat } from 'youtube-chat';
import { PlatformAdapter } from '../base.ts';
import { sanitizeMessage } from '../../utils/sanitizer.ts';

export class YouTubeChatAdapter extends PlatformAdapter {
  private liveChat: LiveChat;
  private identifier: { channelId: string } | { liveId: string };
  
  // Queue to control flow of burst messages
  private messageQueue: any[] = [];
  private isProcessingQueue: boolean = false;
  private readonly processIntervalMs = 300; // 300ms delay between messages
  private retryTimeout: NodeJS.Timeout | null = null;
  private retryCount = 0;

  constructor(identifier: { channelId: string } | { liveId: string }) {
    super('youtube');
    this.identifier = identifier;
    this.liveChat = new LiveChat(identifier);
    this.setupListeners();
  }

  private async processQueue() {
    if (this.isProcessingQueue) return;
    this.isProcessingQueue = true;

    while (this.messageQueue.length > 0) {
      const { chatMessage, user } = this.messageQueue.shift();
      this.emitChat({ msg: chatMessage, user: user, content: chatMessage.content });
      await new Promise(resolve => setTimeout(resolve, this.processIntervalMs));
    }

    this.isProcessingQueue = false;
  }

  private setupListeners() {
    this.liveChat.on('chat', (chatItem) => {
      const user = chatItem.author.name || 'unknown';

      const chatMessage = {
        id: chatItem.id,
        user: user,
        displayName: user,
        content: this.parseMessage(chatItem.message),
        htmlContent: this.parseMessage(chatItem.message),
        color: undefined,
        timestamp: new Date(chatItem.timestamp).toISOString(),
        badges: this.parseBadges(chatItem.author),
      };

      // Queue the message instead of emitting immediately
      this.messageQueue.push({ chatMessage, user });
      this.processQueue();

      // Super Chat detection
      if ('superchat' in chatItem) {
        const sc: any = chatItem.superchat;
        if (sc && sc.amount) {
          this.emitEvent('superchat', {
            user: user,
            amount: sc.amount,
            currency: sc.currency || '',
            message: chatMessage.content
          });
        }
      }
    });

    this.liveChat.on('start', (liveId) => {
      this.logger.info(`Connected to YouTube stream: ${liveId}`);
    });

    this.liveChat.on('error', (err) => {
      this.logger.error('YouTube Chat Error', err);
    });

    this.liveChat.on('end', (reason) => {
      this.logger.info(`YouTube stream ended: ${reason}`);
    });
  }

  private parseMessage(messageRuns: any[]): string {
    if (!Array.isArray(messageRuns)) return '';

    return messageRuns.map(run => {
      if (run.emoji) {
        const url = run.emoji.image?.thumbnails?.[0]?.url;
        if (url) return `<img src="${url}" class="emote" alt="${run.emoji.emojiId || 'emote'}">`;
        return run.text || run.emoji.shortcuts?.[0] || '';
      }

      if (run.url) {
        return `<img src="${run.url}" class="emote" alt="${run.alt || 'emote'}">`;
      }

      return sanitizeMessage(run.text || '');
    }).join('');
  }

  private parseBadges(author: any): any[] {
    const badges = [];
    if (author.isChatOwner) badges.push({ type: 'owner', url: '' });
    if (author.isChatModerator) badges.push({ type: 'moderator', url: '' });
    if (author.isChatSponsor) badges.push({ type: 'subscriber', url: '' });
    return badges;
  }

  async connect(): Promise<void> {
    try {
      const ok = await this.liveChat.start();
      if (!ok) {
        this.logger.error('Failed to start YouTube chat listener');
      } else {
        this.retryCount = 0; // reset on success
      }
    } catch (err: any) {
      const isRateLimited = err?.response?.status === 429 || (err?.message && err.message.includes('429'));
      this.logger.error('Failed to connect to YouTube', isRateLimited ? 'Rate limited (429)' : err);
      
      if (isRateLimited) {
         // Exponential backoff: 2s, 4s, 8s -> max 60s
         const delay = Math.min(2000 * Math.pow(2, this.retryCount), 60000); 
         this.logger.warn(`Retrying YouTube connection in ${delay/1000}s...`);
         this.retryCount++;
         this.retryTimeout = setTimeout(() => this.connect(), delay);
      }
    }
  }

  async disconnect(): Promise<void> {
    if (this.retryTimeout) clearTimeout(this.retryTimeout);
    this.liveChat.stop();
  }
}
