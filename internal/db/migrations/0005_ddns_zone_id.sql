-- FR-31/FR-28~30: caches the Cloudflare zone ID VerifyZoneAccess resolved at
-- registration time, so every later record-management call (creating a
-- subdomain's A record, its SRV record, or updating either on WAN IP
-- change) can skip re-resolving "which zone does this domain belong to"
-- via an extra API round-trip every time.
ALTER TABLE ddns_configs ADD COLUMN zone_id TEXT;
