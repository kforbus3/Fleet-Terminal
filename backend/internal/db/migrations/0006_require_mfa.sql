-- Per-user "require MFA" flag. When set (or when the global require_mfa setting
-- is enabled), a user must have a confirmed second factor; login will not issue
-- a session until they enroll one. MFA remains optional by default.
ALTER TABLE users ADD COLUMN IF NOT EXISTS require_mfa boolean NOT NULL DEFAULT false;
