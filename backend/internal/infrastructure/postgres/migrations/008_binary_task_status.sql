-- У задач остаются только два состояния: не выполнена и выполнена.
UPDATE tasks
SET status = 'todo'
WHERE status = 'doing';

ALTER TABLE tasks
    ALTER COLUMN status SET DEFAULT 'todo';

ALTER TABLE tasks
    DROP CONSTRAINT IF EXISTS tasks_status_check;

ALTER TABLE tasks
    ADD CONSTRAINT tasks_status_check
    CHECK (status IN ('todo', 'done'));
