import { Platform } from "../core/eventBus.ts";

export { Platform };

export interface Badge {
  type: string;
  url: string;
}

export interface ChatMessage {
  id: string;
  platform: Platform;
  user: string;
  displayName: string;
  content: string;
  htmlContent?: string;
  color?: string;
  badges: Badge[];
  timestamp: string;
}

export interface StreamEvent {
  id: string;
  type: string;
  platform: Platform;
  user: string;
  displayName: string;
  timestamp: string;
  data: any;
}
