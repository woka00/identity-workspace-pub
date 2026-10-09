ALTER TABLE work_projects
    ADD COLUMN content_revision BIGINT NOT NULL DEFAULT 1;

CREATE INDEX work_comments_task_created_idx
    ON work_comments(task_id, created_at);

CREATE INDEX work_attachments_project_created_idx
    ON work_attachments(project_id, created_at);

CREATE OR REPLACE FUNCTION bump_work_project_content_revision()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    UPDATE work_projects
    SET content_revision = content_revision + 1
    WHERE id = COALESCE(NEW.project_id, OLD.project_id);
    RETURN COALESCE(NEW, OLD);
END;
$$;

CREATE TRIGGER work_project_members_revision
AFTER INSERT OR UPDATE OR DELETE ON work_project_members
FOR EACH ROW EXECUTE FUNCTION bump_work_project_content_revision();

CREATE TRIGGER work_sections_revision
AFTER INSERT OR UPDATE OR DELETE ON work_sections
FOR EACH ROW EXECUTE FUNCTION bump_work_project_content_revision();

CREATE TRIGGER work_tasks_revision
AFTER INSERT OR UPDATE OR DELETE ON work_tasks
FOR EACH ROW EXECUTE FUNCTION bump_work_project_content_revision();

CREATE TRIGGER work_attachments_revision
AFTER INSERT OR UPDATE OR DELETE ON work_attachments
FOR EACH ROW EXECUTE FUNCTION bump_work_project_content_revision();

CREATE TRIGGER work_links_revision
AFTER INSERT OR UPDATE OR DELETE ON work_links
FOR EACH ROW EXECUTE FUNCTION bump_work_project_content_revision();

CREATE OR REPLACE FUNCTION bump_work_project_revision_from_comment()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    UPDATE work_projects project
    SET content_revision = content_revision + 1
    FROM work_tasks task
    WHERE task.id = NEW.task_id
      AND project.id = task.project_id;
    RETURN NEW;
END;
$$;

CREATE TRIGGER work_comments_revision
AFTER INSERT OR UPDATE ON work_comments
FOR EACH ROW EXECUTE FUNCTION bump_work_project_revision_from_comment();
