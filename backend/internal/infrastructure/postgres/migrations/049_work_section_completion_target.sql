ALTER TABLE work_sections
    ADD COLUMN completed_section_id BIGINT REFERENCES work_sections(id) ON DELETE SET NULL;

ALTER TABLE work_sections
    ADD CONSTRAINT work_sections_completed_section_not_self
    CHECK (completed_section_id IS NULL OR completed_section_id <> id);
