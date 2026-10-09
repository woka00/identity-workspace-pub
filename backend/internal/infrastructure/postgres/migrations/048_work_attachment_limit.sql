ALTER TABLE work_attachments
    DROP CONSTRAINT work_attachments_size_bytes_check;

ALTER TABLE work_attachments
    ADD CONSTRAINT work_attachments_size_bytes_check
    CHECK (size_bytes BETWEEN 1 AND 52428800);
