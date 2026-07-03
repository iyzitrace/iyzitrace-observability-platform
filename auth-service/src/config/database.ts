import sqlite3 from 'sqlite3';
import { open, Database } from 'sqlite';
import path from 'path';

let db: Database | null = null;

interface Migration {
  version: number;
  description: string;
  up: (db: Database) => Promise<void>;
}

const columnExists = async (db: Database, table: string, column: string): Promise<boolean> => {
  const cols = await db.all(`PRAGMA table_info(${table})`);
  return cols.some((c: { name: string }) => c.name === column);
};

const MIGRATIONS: Migration[] = [
  {
    version: 1,
    description: 'baseline: users, api_keys, settings',
    up: async (db) => {
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

      if (!(await columnExists(db, 'api_keys', 'prefix'))) {
        await db.exec('ALTER TABLE api_keys ADD COLUMN prefix TEXT');
      }
      if (!(await columnExists(db, 'api_keys', 'revoked_at'))) {
        await db.exec('ALTER TABLE api_keys ADD COLUMN revoked_at DATETIME');
      }
    }
  },
  {
    version: 2,
    description: 'multitenancy: license, tenants, subtenants, api_keys scope',
    up: async (db) => {
      await db.exec(`
        CREATE TABLE IF NOT EXISTS license (
          id              INTEGER PRIMARY KEY AUTOINCREMENT,
          token           TEXT NOT NULL,
          kid             TEXT,
          account_sub     TEXT NOT NULL,
          customer        TEXT,
          edition         TEXT,
          exp             INTEGER NOT NULL,
          grace_days      INTEGER NOT NULL DEFAULT 0,
          max_tenants     INTEGER NOT NULL,
          max_subtenants  INTEGER NOT NULL,
          features        TEXT NOT NULL DEFAULT '[]',
          binding         TEXT,
          signature_valid INTEGER NOT NULL DEFAULT 0,
          status          TEXT NOT NULL DEFAULT 'active',
          installed_at    DATETIME DEFAULT CURRENT_TIMESTAMP,
          is_active       INTEGER NOT NULL DEFAULT 1
        );

        CREATE TABLE IF NOT EXISTS tenants (
          id          TEXT PRIMARY KEY,
          account_sub TEXT NOT NULL,
          name        TEXT NOT NULL,
          slug        TEXT NOT NULL UNIQUE,
          status      TEXT NOT NULL DEFAULT 'active',
          created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
          deleted_at  DATETIME
        );

        CREATE TABLE IF NOT EXISTS subtenants (
          id          TEXT PRIMARY KEY,
          tenant_id   TEXT NOT NULL REFERENCES tenants(id),
          name        TEXT NOT NULL,
          slug        TEXT NOT NULL,
          org_id      TEXT NOT NULL UNIQUE,
          status      TEXT NOT NULL DEFAULT 'active',
          created_at  DATETIME DEFAULT CURRENT_TIMESTAMP,
          deleted_at  DATETIME,
          UNIQUE (tenant_id, slug)
        );
      `);

      if (!(await columnExists(db, 'api_keys', 'tenant_id'))) {
        await db.exec('ALTER TABLE api_keys ADD COLUMN tenant_id TEXT');
      }
      if (!(await columnExists(db, 'api_keys', 'subtenant_id'))) {
        await db.exec('ALTER TABLE api_keys ADD COLUMN subtenant_id TEXT');
      }

      await db.exec('CREATE INDEX IF NOT EXISTS idx_api_keys_prefix ON api_keys(prefix)');
      await db.exec('CREATE INDEX IF NOT EXISTS idx_api_keys_subtenant ON api_keys(subtenant_id)');
      await db.exec('CREATE INDEX IF NOT EXISTS idx_subtenants_tenant ON subtenants(tenant_id)');
    }
  },
  {
    version: 3,
    description: 'license: optional per-signal (traces/logs) limits',
    up: async (db) => {
      if (!(await columnExists(db, 'license', 'signal_limits'))) {
        await db.exec("ALTER TABLE license ADD COLUMN signal_limits TEXT NOT NULL DEFAULT '{}'");
      }
    }
  }
];

const runMigrations = async (db: Database): Promise<void> => {
  await db.exec(`
    CREATE TABLE IF NOT EXISTS schema_migrations (
      version     INTEGER PRIMARY KEY,
      description TEXT NOT NULL,
      applied_at  DATETIME DEFAULT CURRENT_TIMESTAMP
    );
  `);

  const appliedRows = await db.all('SELECT version FROM schema_migrations');
  const applied = new Set(appliedRows.map((r: { version: number }) => r.version));

  for (const migration of MIGRATIONS) {
    if (applied.has(migration.version)) continue;

    await db.exec('BEGIN');
    try {
      await migration.up(db);
      await db.run(
        'INSERT INTO schema_migrations (version, description) VALUES (?, ?)',
        migration.version,
        migration.description
      );
      await db.exec('COMMIT');
      console.log(`Applied migration ${migration.version}: ${migration.description}`);
    } catch (err) {
      await db.exec('ROLLBACK');
      throw err;
    }
  }
};

export const initDB = async (): Promise<Database> => {
  if (db) return db;

  const dbPath = process.env.DB_PATH || path.join(__dirname, '../../data/auth.db');

  db = await open({
    filename: dbPath,
    driver: sqlite3.Database
  });

  await db.exec('PRAGMA foreign_keys = ON');
  await runMigrations(db);

  console.log('Database initialized at', dbPath);
  return db;
};

export const getDB = (): Database => {
  if (!db) throw new Error('Database not initialized');
  return db;
};
