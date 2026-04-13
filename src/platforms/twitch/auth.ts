import { db } from "../../core/database.ts";
import { createLogger } from "../../core/logger.ts";

const logger = createLogger("Auth:Twitch");

export interface TokenData {
  accessToken: string;
  refreshToken: string | null;
  expiresIn: number; // Seconds
  obtainmentTimestamp: number;
}

export class TwitchAuthManager {
  private clientId: string;

  constructor(clientId: string) {
    this.clientId = clientId;
  }

  // Get current token from DB
  getToken(): TokenData | null {
    const row = db.query<any, any>("SELECT * FROM tokens WHERE platform = 'twitch'").get();
    if (!row) return null;

    return {
      accessToken: row.access_token,
      refreshToken: row.refresh_token,
      expiresIn: Math.floor((row.expires_at - Date.now()) / 1000),
      obtainmentTimestamp: row.expires_at - (row.expires_at - Date.now()) // Approximation
    };
  }

  // Save token from Twurple or manual flow
  saveToken(tokenData: TokenData) {
    const expiresAt = Date.now() + tokenData.expiresIn * 1000;
    db.run(
      "INSERT OR REPLACE INTO tokens (platform, access_token, refresh_token, expires_at) VALUES (?, ?, ?, ?)",
      ["twitch", tokenData.accessToken, tokenData.refreshToken, expiresAt]
    );
    logger.info("Twitch tokens updated in database");
  }

  // Headless Device Code Grant Flow
  async requestDeviceCode(scopes: string[]): Promise<{ deviceCode: string; userCode: string; verificationUri: string; interval: number; expiresIn: number }> {
    const response = await fetch('https://id.twitch.tv/oauth2/device', {
      method: 'POST',
      headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
      body: new URLSearchParams({
        client_id: this.clientId,
        scopes: scopes.join(' '),
      })
    });

    if (!response.ok) {
      throw new Error(`Device code request failed: ${await response.text()}`);
    }

    const data = await response.json();
    return {
      deviceCode: data.device_code,
      userCode: data.user_code,
      verificationUri: data.verification_uri,
      interval: data.interval,
      expiresIn: data.expires_in
    };
  }

  // Poll for token after user inputs the Device Code
  async pollForToken(deviceCode: string, interval: number, maxAttempts: number = 60): Promise<TokenData> {
    let attempts = 0;

    return new Promise((resolve, reject) => {
      const poll = async () => {
        attempts++;
        if (attempts > maxAttempts) {
          return reject(new Error("Polling timed out"));
        }

        try {
          const response = await fetch('https://id.twitch.tv/oauth2/token', {
            method: 'POST',
            headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
            body: new URLSearchParams({
              client_id: this.clientId,
              device_code: deviceCode,
              grant_type: 'urn:ietf:params:oauth:grant-type:device_code'
            })
          });

          const data = await response.json();

          if (response.ok) {
            const tokenData: TokenData = {
              accessToken: data.access_token,
              refreshToken: data.refresh_token,
              expiresIn: data.expires_in,
              obtainmentTimestamp: Date.now()
            };
            this.saveToken(tokenData);
            return resolve(tokenData);
          } else if (data.message === "authorization_pending") {
            // Keep polling
            setTimeout(poll, interval * 1000);
          } else {
            return reject(new Error(`Polling error: ${JSON.stringify(data)}`));
          }
        } catch (err) {
          return reject(err);
        }
      };

      setTimeout(poll, interval * 1000);
    });
  }
}
