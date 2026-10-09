-- Rejected moderation candidates remain available in their owner's private
-- catalogue, but no longer appear in the administrator review queue.
CREATE TABLE food_catalog_rejections (
    source_food_id      BIGINT PRIMARY KEY REFERENCES food_catalog(id) ON DELETE CASCADE,
    rejected_by_user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    rejected_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
