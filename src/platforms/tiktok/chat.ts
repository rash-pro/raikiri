import { WebcastPushConnection } from 'tiktok-live-connector';
import { PlatformAdapter } from '../base.ts';
import { sanitizeMessage } from '../../utils/sanitizer.ts';

export class TikTokChatAdapter extends PlatformAdapter {
  private channelName: string;
  private connection: WebcastPushConnection | null = null;

  constructor(channelName: string) {
    super('tiktok');
    this.channelName = channelName;
  }

  async connect(): Promise<void> {
    try {
      this.logger.info(`Connecting to TikTok live: ${this.channelName}...`);
      this.connection = new WebcastPushConnection(this.channelName);

      this.connection.connect().then(state => {
        this.logger.info(`Connected to TikTok room: ${state.roomId}`);
      }).catch(err => {
        this.logger.error('TikTok connection failed:', err);
      });

      this.setupListeners();
    } catch (error: any) {
      this.logger.error('Failed to initialize TikTok connection:', error);
    }
  }

  private setupListeners() {
    if (!this.connection) return;

    this.connection.on('chat', (data) => {
      this.handleMessage(data);
    });

    // TikTok events
    this.connection.on('gift', (data) => {
      this.emitEvent('gift', {
        user: data.uniqueId,
        giftName: data.giftName,
        diamondValue: data.diamondCount * data.repeatCount,
        count: data.repeatCount
      });
    });

    this.connection.on('like', (data) => {
      this.emitEvent('like', {
        user: data.uniqueId,
        totalLikes: data.totalLikeCount
      });
    });

    this.connection.on('social', (data) => {
      if (data.displayType === 'pm_mt_msg_viewer_is_follower') {
        this.emitEvent('follow', { user: data.uniqueId });
      } else if (data.displayType === 'pm_main_bln_share_a_person' || data.label?.includes('share')) {
        this.emitEvent('share', { user: data.uniqueId });
      }
    });

    this.connection.on('error', (err) => {
      this.logger.error('TikTok error:', err);
    });

    this.connection.on('disconnected', () => {
      this.logger.warn('TikTok disconnected');
    });
  }

  private handleMessage(data: any) {
    const user = data.uniqueId;

    const chatMessage = {
      id: data.msgId,
      platform: this.platform,
      user: user,
      displayName: data.nickname,
      content: sanitizeMessage(data.comment),
      htmlContent: sanitizeMessage(data.comment),
      color: '#ff0050',
      badges: this.normalizeBadges(data.badges),
      timestamp: new Date().toISOString()
    };

    this.emitChat({ msg: chatMessage, user: user, content: data.comment });
  }

  private normalizeBadges(badges: any[]): any[] {
    const normalized: any[] = [];
    if (!badges) return normalized;

    badges.forEach(b => {
      const name = b.name ? b.name.toLowerCase() : '';
      if (name.includes('moderator')) normalized.push({ type: 'moderator', url: '' });
      if (name.includes('subscriber')) normalized.push({ type: 'subscriber', url: '' });
    });

    return normalized;
  }

  async disconnect(): Promise<void> {
    if (this.connection) {
      this.connection.disconnect();
      this.connection = null;
    }
  }
}
