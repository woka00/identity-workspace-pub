-- FatSecret OAuth connection for the original single-user schema.
CREATE TABLE IF NOT EXISTS fatsecret_connection (
    id SMALLINT PRIMARY KEY CHECK (id = 1),
    oauth_token TEXT NOT NULL,
    oauth_token_secret TEXT NOT NULL,
    connected_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS fatsecret_oauth_requests (
    oauth_token TEXT PRIMARY KEY,
    oauth_token_secret TEXT NOT NULL,
    return_to TEXT NOT NULL DEFAULT '/',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS fatsecret_oauth_requests_created_at_idx
    ON fatsecret_oauth_requests (created_at);
