-- FR-33a: surfaces a Let's Encrypt renewal failure to the operator ahead of
-- expiry, instead of only finding out once the certificate has actually
-- expired and browsers start rejecting it. Only meaningful for
-- kind=main_domain (the only path with a real certmagic-managed
-- certificate at all -- see internal/tlscert.Manager).
ALTER TABLE ddns_configs ADD COLUMN cert_renewal_error TEXT;
ALTER TABLE ddns_configs ADD COLUMN cert_renewal_error_at TEXT;
