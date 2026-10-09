-- Match the project task query order. The older index keeps section_id between
-- project_id and parent_id, so it cannot provide this ordering efficiently.
CREATE INDEX work_tasks_project_tree_order_idx
    ON work_tasks(project_id, parent_id, sort_order, id);

-- Note previews are read far more often than bodies are edited. Materialize the
-- normalized preview on writes instead of running regexp_replace over every
-- note each time the project resource panel is opened.
ALTER TABLE work_notes
    ADD COLUMN preview TEXT NOT NULL DEFAULT '';

UPDATE work_notes
SET preview = left(regexp_replace(btrim(body), '[[:space:]]+', ' ', 'g'), 240);

CREATE OR REPLACE FUNCTION set_work_note_preview()
RETURNS trigger
LANGUAGE plpgsql
AS $$
BEGIN
    NEW.preview := left(regexp_replace(btrim(NEW.body), '[[:space:]]+', ' ', 'g'), 240);
    RETURN NEW;
END;
$$;

CREATE TRIGGER work_notes_preview
BEFORE INSERT OR UPDATE OF body ON work_notes
FOR EACH ROW EXECUTE FUNCTION set_work_note_preview();

-- pinned is a per-user presentation preference. Changing it must not force all
-- collaborators to reload the project contents.
DROP TRIGGER work_project_members_revision ON work_project_members;
CREATE TRIGGER work_project_members_revision
AFTER INSERT OR DELETE OR UPDATE OF role ON work_project_members
FOR EACH ROW EXECUTE FUNCTION bump_work_project_content_revision();
