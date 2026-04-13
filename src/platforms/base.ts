import { Platform } from "./types.ts";
import { createLogger } from "../core/logger.ts";
import { eventBus } from "../core/eventBus.ts";

export abstract class PlatformAdapter {
  public platform: Platform;
  protected logger: ReturnType<typeof createLogger>;

  constructor(platform: Platform) {
    this.platform = platform;
    this.logger = createLogger(`Adapter:${platform}`);
  }

  abstract connect(): Promise<void>;
  abstract disconnect(): Promise<void>;

  protected emitChat(data: any) {
    eventBus.emitEvent('chat', { type: 'chat', platform: this.platform, ...data });
  }

  protected emitEvent(eventType: string, data: any) {
    eventBus.emitEvent(eventType, { type: eventType, platform: this.platform, ...data });
  }
}
