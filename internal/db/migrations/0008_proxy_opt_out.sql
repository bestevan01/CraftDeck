-- ReconcileProxyMode (daemon startup + domain registration changes) used to
-- treat "not currently a proxy backend" as always meaning "never decided
-- yet" and auto-register it whenever a main domain is registered -- which
-- silently undid an operator's explicit "convert to independent exposure"
-- choice on every daemon restart (e.g. after a self-update). This column
-- records that explicit choice so ReconcileProxyMode can tell the two
-- apart and leave opted-out servers alone.
ALTER TABLE instances ADD COLUMN proxy_opt_out INTEGER NOT NULL DEFAULT 0;
