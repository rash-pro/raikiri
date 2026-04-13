import { EventEmitter } from 'node:events';

export type Platform = 'twitch' | 'youtube' | 'kick' | 'tiktok';

export interface ChatMessageData {
  platform: Platform;
  user: string;
  displayName: string;
  content: string;
  htmlContent?: string;
  color?: string;
  badges: any[];
  timestamp: string;
}

export type RaikiriEvent =
  | { type: 'chat'; platform: Platform; user: string; content: string; msg: ChatMessageData }
  | { type: 'follow'; platform: Platform; user: string }
  | { type: 'subscription'; platform: Platform; user: string; tier?: number; months?: number; message?: string }
  | { type: 'gift'; platform: Platform; user: string; giftName?: string; diamondValue?: number; count?: number }
  | { type: 'superchat'; platform: Platform; user: string; amount: number; currency: string; message: string }
  | { type: 'raid'; platform: Platform; user: string; viewers: number }
  | { type: 'bits'; platform: Platform; user: string; amount: number; message?: string }
  | { type: 'like'; platform: Platform; user: string; totalLikes: number }
  | { type: 'share'; platform: Platform; user: string }
  | { type: 'system'; message: string };

class TypedEventBus extends EventEmitter {
  emitEvent(eventName: string, data: any) {
    this.emit(eventName, data);
  }

  onEvent(eventName: string, listener: (data: any) => void) {
    this.on(eventName, listener);
  }
}

export const eventBus = new TypedEventBus();
