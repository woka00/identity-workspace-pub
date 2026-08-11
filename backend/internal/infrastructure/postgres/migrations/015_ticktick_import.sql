-- Mark which side originally created a linked task. Existing links were created
-- by the local application before inbound synchronization existed, so they keep
-- the legacy "avatar" origin value used by application code.
ALTER TABLE user_ticktick_task_links
    ADD COLUMN IF NOT EXISTS source TEXT NOT NULL DEFAULT 'avatar';

ALTER TABLE user_ticktick_task_links
    DROP CONSTRAINT IF EXISTS user_ticktick_task_links_source_check;

ALTER TABLE user_ticktick_task_links
    ADD CONSTRAINT user_ticktick_task_links_source_check
    CHECK (source IN ('avatar', 'ticktick'));
