import { StaticAuthProvider } from '@twurple/auth';
import { ApiClient } from '@twurple/api';
import { EventSubWsListener } from '@twurple/eventsub-ws';
import { PlatformAdapter } from '../base.ts';
import { TwitchAuthManager } from './auth.ts';

export class TwitchEventAdapter extends PlatformAdapter {
  private listener: EventSubWsListener | null = null;
  private apiClient: ApiClient | null = null;
  private authManager: TwitchAuthManager;
  private clientId: string;
  private channelName: string;

  constructor(clientId: string, clientSecret: string, channelName: string) {
    super('twitch');
    this.clientId = clientId;
    this.channelName = channelName;
    this.authManager = new TwitchAuthManager(clientId);
  }

  async connect(): Promise<void> {
    const initialToken = this.authManager.getToken();
    if (!initialToken || !initialToken.accessToken) {
      this.logger.warn("No Twitch token found. Cannot start EventSub. Please authenticate via Dashboard.");
      return;
    }

    try {
      const authProvider = new StaticAuthProvider(this.clientId, initialToken.accessToken);
      this.apiClient = new ApiClient({ authProvider });
      
      const user = await this.apiClient.users.getUserByName(this.channelName);
      if (!user) {
         throw new Error(`User ${this.channelName} not found`);
      }

      this.listener = new EventSubWsListener({ apiClient: this.apiClient });
      await this.listener.start();
      
      this.setupSubscriptions(user.id);
      this.logger.info(`Started EventSub for Twitch channel: ${this.channelName}`);
    } catch (e: any) {
      this.logger.error("Failed to start Twitch EventSub", e);
    }
  }

  private async setupSubscriptions(userId: string) {
    if (!this.listener) return;

    // Follows
    this.listener.onChannelFollow(userId, userId, (e) => {
      this.emitEvent('follow', { user: e.userDisplayName });
    });

    // Subscriptions
    this.listener.onChannelSubscription(userId, (e) => {
      this.emitEvent('subscription', {
        user: e.userDisplayName,
        tier: parseInt(e.tier) / 1000,
        message: ''
      });
    });

    // Sub Messages (Resubs)
    this.listener.onChannelSubscriptionMessage(userId, (e) => {
      this.emitEvent('subscription', {
        user: e.userDisplayName,
        tier: parseInt(e.tier) / 1000,
        months: e.cumulativeMonths,
        message: e.messageText
      });
    });

    // Sub Gifts
    this.listener.onChannelSubscriptionGift(userId, (e) => {
      this.emitEvent('gift', {
        user: e.gifterDisplayName,
        giftName: `Tier ${parseInt(e.tier) / 1000} Sub`,
        count: e.amount
      });
    });

    // Bits (Cheering)
    this.listener.onChannelCheer(userId, (e) => {
      this.emitEvent('bits', {
        user: e.userDisplayName,
        amount: e.bits,
        message: e.message
      });
    });

    // Raids
    this.listener.onChannelRaidTo(userId, (e) => {
      this.emitEvent('raid', {
        user: e.raidingBroadcasterDisplayName,
        viewers: e.viewers
      });
    });

    // Channel Points Redemptions
    this.listener.onChannelRedemptionAdd(userId, (e) => {
      if (!e.input) return; // Only care about rewards with text input for TTS
      
      this.emitEvent('channel_points', {
        user: e.userDisplayName,
        rewardTitle: e.rewardTitle,
        rewardId: e.rewardId,
        message: e.input
      });
    });
  }

  async disconnect(): Promise<void> {
    if (this.listener) {
      this.listener.stop();
      this.listener = null;
    }
  }
}
