-- Existing preview accounts keep their current access and do not acquire a
-- fictitious verified email. Public accounts are inserted only after verification.
ALTER TABLE users ADD COLUMN email_encrypted TEXT;
ALTER TABLE users ADD COLUMN email_hash CHAR(64) UNIQUE;
ALTER TABLE users ADD COLUMN email_verified_at TIMESTAMPTZ;
ALTER TABLE users ADD CONSTRAINT users_verified_email_consistency CHECK (
    (email_encrypted IS NULL AND email_hash IS NULL AND email_verified_at IS NULL)
    OR (email_encrypted IS NOT NULL AND email_encrypted LIKE 'enc:v2:%' AND email_hash IS NOT NULL AND email_verified_at IS NOT NULL)
);

CREATE TABLE registration_requests (
    email_hash CHAR(64) PRIMARY KEY,
    email_encrypted TEXT NOT NULL CHECK (email_encrypted LIKE 'enc:v2:%'),
    token_hash CHAR(64) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX registration_requests_expiry_idx ON registration_requests (expires_at);

-- Durable quotas shared by replicas; no plaintext email or token is recorded.
CREATE TABLE registration_mail_attempts (
    id BIGSERIAL PRIMARY KEY,
    email_hash CHAR(64) NOT NULL,
    attempted_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX registration_mail_attempts_time_idx ON registration_mail_attempts (attempted_at);
CREATE INDEX registration_mail_attempts_email_idx ON registration_mail_attempts (email_hash, attempted_at);
