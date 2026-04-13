import { Database } from "bun:sqlite";
import fs from "fs";
import path from "path";

const dataDir = path.join(process.cwd(), "data");
if (!fs.existsSync(dataDir)) {
  fs.mkdirSync(dataDir, { recursive: true });
}

export const db = new Database(path.join(dataDir, "raikiri.db"));

// Optimize DB performance
db.run("PRAGMA journal_mode = WAL");
db.run("PRAGMA synchronous = NORMAL");

// Initialize tables
db.run(`
  CREATE TABLE IF NOT EXISTS config (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
  )
`);

db.run(`
  CREATE TABLE IF NOT EXISTS tokens (
    platform TEXT PRIMARY KEY,
    access_token TEXT NOT NULL,
    refresh_token TEXT,
    expires_at INTEGER,
    scope TEXT
  )
`);

db.run(`
  CREATE TABLE IF NOT EXISTS events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    platform TEXT NOT NULL,
    data TEXT NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
  )
`);

export const configDb = {
  get: (key: string): string | null => {
    const row = db.query<{ value: string }, { $key: string }>("SELECT value FROM config WHERE key = $key").get({ $key: key });
    return row ? row.value : null;
  },
  set: (key: string, value: string) => {
    db.run("INSERT OR REPLACE INTO config (key, value) VALUES ($key, $value)", { $key: key, $value: value });
  },
  delete: (key: string) => {
    db.run("DELETE FROM config WHERE key = $key", { $key: key });
  }
};
