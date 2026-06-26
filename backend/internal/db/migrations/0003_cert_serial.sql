-- Monotonic serial source for issued SSH certificates. Each certificate gets a
-- unique, never-reused serial used for revocation lists (KRL) and audit.
CREATE SEQUENCE IF NOT EXISTS ssh_cert_serial_seq AS BIGINT START WITH 1 INCREMENT BY 1;
