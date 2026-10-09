CREATE TABLE work_projects (
    id BIGSERIAL PRIMARY KEY,
    legacy_goal_id BIGINT UNIQUE REFERENCES goals(id) ON DELETE SET NULL,
    owner_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    summary TEXT NOT NULL DEFAULT '',
    legacy_current_value DOUBLE PRECISION,
    legacy_target_value DOUBLE PRECISION,
    status TEXT NOT NULL DEFAULT 'on_track' CHECK (status IN ('on_track','at_risk','off_track')),
    deadline DATE,
    completed_at TIMESTAMPTZ,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE work_project_members (
    project_id BIGINT NOT NULL REFERENCES work_projects(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role TEXT NOT NULL CHECK (role IN ('owner','member')),
    pinned BOOLEAN NOT NULL DEFAULT FALSE,
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id)
);
CREATE UNIQUE INDEX work_project_one_owner_idx ON work_project_members(project_id) WHERE role='owner';
CREATE INDEX work_project_members_user_idx ON work_project_members(user_id, project_id);

CREATE TABLE work_sections (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES work_projects(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    sort_order BIGINT NOT NULL DEFAULT 0
);
CREATE INDEX work_sections_order_idx ON work_sections(project_id, sort_order, id);

CREATE TABLE work_tasks (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES work_projects(id) ON DELETE CASCADE,
    section_id BIGINT REFERENCES work_sections(id) ON DELETE SET NULL,
    parent_id BIGINT REFERENCES work_tasks(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    assignee_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    due_date DATE,
    completed_at TIMESTAMPTZ,
    sort_order BIGINT NOT NULL DEFAULT 0,
    version BIGINT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (parent_id IS NULL OR section_id IS NULL)
);
CREATE INDEX work_tasks_order_idx ON work_tasks(project_id, section_id, parent_id, sort_order, id);

CREATE TABLE work_comments (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT NOT NULL REFERENCES work_tasks(id) ON DELETE CASCADE,
    author_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE work_attachments (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES work_projects(id) ON DELETE CASCADE,
    task_id BIGINT REFERENCES work_tasks(id) ON DELETE CASCADE,
    uploader_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    mime_type TEXT NOT NULL DEFAULT 'application/octet-stream',
    size_bytes BIGINT NOT NULL CHECK (size_bytes BETWEEN 1 AND 20971520),
    content BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TABLE work_events (
    id BIGSERIAL PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES work_projects(id) ON DELETE CASCADE,
    actor_id BIGINT REFERENCES users(id) ON DELETE SET NULL,
    kind TEXT NOT NULL,
    message TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX work_events_project_idx ON work_events(project_id, created_at DESC, id DESC);
CREATE TABLE work_invites (
    token_hash CHAR(64) PRIMARY KEY,
    project_id BIGINT NOT NULL REFERENCES work_projects(id) ON DELETE CASCADE,
    created_by BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO work_projects (legacy_goal_id, owner_id, title, description, summary, legacy_current_value, legacy_target_value, deadline, completed_at, created_at, updated_at)
SELECT id, user_id, title, description, summary, current_value, target_value, deadline, completed_at, created_at, updated_at
FROM goals WHERE user_id IS NOT NULL ON CONFLICT (legacy_goal_id) DO NOTHING;
INSERT INTO work_project_members (project_id, user_id, role, pinned)
SELECT wp.id, wp.owner_id, 'owner', g.pinned FROM work_projects wp JOIN goals g ON g.id=wp.legacy_goal_id
ON CONFLICT (project_id,user_id) DO NOTHING;
INSERT INTO work_sections (project_id, title, sort_order)
SELECT id, 'Без раздела', 0 FROM work_projects;
INSERT INTO work_tasks (project_id, section_id, title, description, due_date, completed_at, sort_order, created_at, updated_at)
SELECT wp.id, ws.id, t.title, t.description, t.due_date, t.completed_at, t.sort_order, t.created_at, t.created_at
FROM work_projects wp
JOIN goals g ON g.id=wp.legacy_goal_id
JOIN LATERAL jsonb_array_elements_text(g.related_task_ids) related(id) ON true
JOIN tasks t ON t.id::text=related.id AND t.user_id=g.user_id
JOIN work_sections ws ON ws.project_id=wp.id AND ws.title='Без раздела';
