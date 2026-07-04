-- Schema per ARCHITECTURE.md section 3. Kept as a single migration until
-- the first release ships; split into incremental files after that point.

CREATE TABLE users (
  id INTEGER PRIMARY KEY,
  username TEXT UNIQUE NOT NULL,
  password_hash TEXT NOT NULL,
  totp_secret TEXT,
  totp_enabled INTEGER NOT NULL DEFAULT 0,
  backup_codes_json TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE sessions (
  id TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id),
  expires_at TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE instances (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  kind TEXT NOT NULL CHECK (kind IN ('server','proxy')),
  loader TEXT NOT NULL,
  loader_version TEXT,
  mc_version TEXT,
  java_major INTEGER,
  game_port INTEGER,
  rcon_port INTEGER,
  rcon_password TEXT NOT NULL,
  cpu_quota_percent INTEGER,
  memory_max_mb INTEGER,
  work_dir TEXT NOT NULL,
  status TEXT NOT NULL,
  created_at TEXT NOT NULL
);

CREATE TABLE proxy_backends (
  proxy_id TEXT NOT NULL REFERENCES instances(id),
  backend_instance_id TEXT NOT NULL REFERENCES instances(id),
  priority INTEGER NOT NULL,
  forced_host TEXT,
  PRIMARY KEY (proxy_id, backend_instance_id)
);

CREATE TABLE plugins (
  id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL REFERENCES instances(id),
  source TEXT NOT NULL CHECK (source IN ('modrinth','upload')),
  modrinth_project_id TEXT,
  modrinth_version_id TEXT,
  filename TEXT NOT NULL,
  sha512 TEXT,
  enabled INTEGER NOT NULL DEFAULT 1,
  installed_as_dependency INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL
);

CREATE TABLE backups (
  id TEXT PRIMARY KEY,
  instance_id TEXT NOT NULL REFERENCES instances(id),
  filename TEXT NOT NULL,
  size_bytes INTEGER,
  created_at TEXT NOT NULL
);

CREATE TABLE port_mappings (
  id TEXT PRIMARY KEY,
  instance_id TEXT REFERENCES instances(id),
  external_port INTEGER NOT NULL,
  internal_port INTEGER NOT NULL,
  protocol TEXT NOT NULL CHECK (protocol IN ('tcp','udp')),
  method TEXT NOT NULL CHECK (method IN ('upnp','natpmp','manual')),
  created_at TEXT NOT NULL
);

CREATE TABLE ddns_configs (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL CHECK (kind IN ('free_subdomain','main_domain')),
  provider TEXT NOT NULL,
  hostname TEXT NOT NULL,
  mode TEXT NOT NULL CHECK (mode IN ('active','monitor')),
  credentials_encrypted TEXT,
  last_known_ip TEXT,
  last_checked_at TEXT,
  created_at TEXT NOT NULL
);

CREATE TABLE domain_assignments (
  ddns_config_id TEXT NOT NULL REFERENCES ddns_configs(id),
  instance_id TEXT NOT NULL REFERENCES instances(id),
  subdomain TEXT NOT NULL,
  srv_port INTEGER NOT NULL,
  PRIMARY KEY (ddns_config_id, subdomain)
);
