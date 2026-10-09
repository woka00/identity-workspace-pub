ALTER TABLE work_sections
    ADD COLUMN reset_completed_on_move BOOLEAN NOT NULL DEFAULT FALSE;
