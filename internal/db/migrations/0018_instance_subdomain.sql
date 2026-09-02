-- A subdomain for a server that is NOT behind the Velocity proxy.
--
-- Until now the only place a subdomain could live was
-- proxy_backends.forced_host, which by definition only exists for servers
-- sitting behind the proxy -- so an independently-exposed server (the only
-- option for NeoForge/Forge, which Velocity does not support at all) could
-- never be reached by name, only by "domain:port". This column is the
-- independent-exposure counterpart to forced_host.
--
-- The two are deliberately kept separate rather than merged into one
-- column: forced_host is a Velocity *routing key* written into
-- velocity.toml, while this is purely a DNS name. A server is either
-- behind the proxy or independent -- never both -- so whichever field
-- applies is unambiguous at any moment (see handlers_proxy.go's
-- serverSubdomain).
--
-- The DNS records the two produce differ as well: a forced_host gets an A
-- record only (the proxy already listens on the default port 25565, and
-- routes by the hostname in the client's handshake), while this gets an A
-- record *plus* an SRV record pointing at the instance's own game_port,
-- which is what lets players omit the port (see SyncMainDomainDNS).
ALTER TABLE instances ADD COLUMN subdomain TEXT NOT NULL DEFAULT '';

-- Two servers pointing the same name at different ports would make the
-- name resolve to whichever record won the last sync -- a silent,
-- confusing failure. Partial so the empty default (no subdomain assigned)
-- stays repeatable across every instance.
CREATE UNIQUE INDEX idx_instances_subdomain ON instances(subdomain) WHERE subdomain != '';
