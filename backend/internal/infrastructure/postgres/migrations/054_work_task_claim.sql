ALTER TABLE work_tasks
    ADD COLUMN created_by BIGINT REFERENCES users(id) ON DELETE SET NULL;

UPDATE work_tasks task
SET created_by = project.owner_id
FROM work_projects project
WHERE project.id = task.project_id
  AND task.created_by IS NULL;
