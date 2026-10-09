CREATE INDEX work_tasks_parent_idx
    ON work_tasks(parent_id)
    WHERE parent_id IS NOT NULL;

CREATE INDEX work_attachments_task_idx
    ON work_attachments(task_id)
    WHERE task_id IS NOT NULL;
