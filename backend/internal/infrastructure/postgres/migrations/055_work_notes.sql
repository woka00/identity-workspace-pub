CREATE TABLE work_notes (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES work_projects(id) ON DELETE CASCADE,
    created_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    title TEXT NOT NULL,
    body TEXT NOT NULL DEFAULT '',
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX work_notes_project_updated_idx
    ON work_notes(project_id, updated_at DESC, id DESC);

CREATE TRIGGER work_notes_revision
AFTER INSERT OR UPDATE OR DELETE ON work_notes
FOR EACH ROW EXECUTE FUNCTION bump_work_project_content_revision();
