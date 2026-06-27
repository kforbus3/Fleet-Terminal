-- The management address operators enter to reach a host during enrollment may
-- be a hostname or an IP, so it cannot be constrained to INET. The WireGuard
-- overlay address remains INET (always an IP).
ALTER TABLE hosts ALTER COLUMN address TYPE TEXT USING host(address);
