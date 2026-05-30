import sqlite3 from 'sqlite3';
import { open, Database } from 'sqlite';
import path from 'path';

let db: Database | null = null;

export const initDB = async (): Promise<Database> => {
  if (db) return db;

  const dbPath = process.env.DB_PATH || path.join(__dirname, '../../data/auth.db');

  db = await open({
    filename: dbPath,
    driver: sqlite3.Database
  });

  await db.exec(`
    CREATE TABLE IF NOT EXISTS users (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      username TEXT UNIQUE NOT NULL,
      password_hash TEXT NOT NULL,
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP
    );

    CREATE TABLE IF NOT EXISTS api_keys (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      name TEXT NOT NULL,
      prefix TEXT,
      key_hash TEXT UNIQUE NOT NULL,
      role TEXT DEFAULT 'editor',
      created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
      revoked_at DATETIME
    );

    CREATE TABLE IF NOT EXISTS settings (
      key TEXT PRIMARY KEY,
      value TEXT NOT NULL
    );
    
    INSERT OR IGNORE INTO settings (key, value) VALUES ('security.agent.api_key', 'false');
    INSERT OR IGNORE INTO settings (key, value) VALUES ('security.external.api_key', 'false');
    INSERT OR IGNORE INTO settings (key, value) VALUES ('security.force_ssl', 'false');
  `);

  // Migration for existing tables
  try {
    await db.exec('ALTER TABLE api_keys ADD COLUMN prefix TEXT');
  } catch (e) { }
  try {
    await db.exec('ALTER TABLE api_keys ADD COLUMN revoked_at DATETIME');
  } catch (e) { }

  console.log('Database initialized at', dbPath);
  return db;
};

export const getDB = (): Database => {
  if (!db) throw new Error('Database not initialized');
  return db;
};
