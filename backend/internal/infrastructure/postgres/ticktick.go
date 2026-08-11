package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"identity-workspace/internal/domain"
)

func (s *Repository) TickTickConnection(ctx context.Context) (domain.TickTickConnection, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.TickTickConnection{}, err
	}
	var connection domain.TickTickConnection
	err = s.db.QueryRowContext(ctx, `
		SELECT access_token, project_id, project_name,
		       to_char(connected_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')
		FROM user_ticktick_connections WHERE user_id=$1`, userID).Scan(
		&connection.AccessToken, &connection.ProjectID, &connection.ProjectName, &connection.ConnectedAt,
	)
	if err == sql.ErrNoRows {
		return domain.TickTickConnection{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.TickTickConnection{}, err
	}
	connection.AccessToken, err = s.decryptSecret(tickTickTokenPurpose(userID), connection.AccessToken)
	if err != nil {
		return domain.TickTickConnection{}, err
	}
	return connection, nil
}

func (s *Repository) SaveTickTickOAuthState(ctx context.Context, pending domain.TickTickOAuthState) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		DELETE FROM user_ticktick_oauth_states
		WHERE created_at < now() - interval '20 minutes' OR user_id=$1`, userID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO user_ticktick_oauth_states (state, user_id, callback_url, return_to)
		VALUES ($1, $2, $3, $4)`, pending.State, userID, pending.CallbackURL, pending.ReturnTo)
	return err
}

func (s *Repository) ConsumeTickTickOAuthState(ctx context.Context, state string) (domain.TickTickOAuthState, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.TickTickOAuthState{}, err
	}
	defer tx.Rollback()

	var pending domain.TickTickOAuthState
	err = tx.QueryRowContext(ctx, `
		DELETE FROM user_ticktick_oauth_states
		WHERE state=$1 AND created_at >= now() - interval '20 minutes'
		RETURNING user_id, state, callback_url, return_to`, state).Scan(
		&pending.UserID, &pending.State, &pending.CallbackURL, &pending.ReturnTo,
	)
	if err == sql.ErrNoRows {
		return domain.TickTickOAuthState{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.TickTickOAuthState{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.TickTickOAuthState{}, err
	}
	return pending, nil
}

func (s *Repository) SaveTickTickConnection(ctx context.Context, userID int64, accessToken, projectID, projectName string) error {
	encryptedToken, err := s.encryptSecret(tickTickTokenPurpose(userID), accessToken)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO user_ticktick_connections (user_id, access_token, project_id, project_name, connected_at)
		VALUES ($1, $2, $3, $4, now())
		ON CONFLICT (user_id) DO UPDATE
		SET access_token=EXCLUDED.access_token,
		    project_id=EXCLUDED.project_id,
		    project_name=EXCLUDED.project_name,
		    connected_at=now()`, userID, encryptedToken, projectID, projectName)
	return err
}

func (s *Repository) DeleteTickTickConnection(ctx context.Context) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_ticktick_task_links WHERE user_id=$1`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_ticktick_connections WHERE user_id=$1`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Repository) TickTickTaskLink(ctx context.Context, taskID int64) (domain.TickTickTaskLink, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.TickTickTaskLink{}, err
	}
	var link domain.TickTickTaskLink
	err = s.db.QueryRowContext(ctx, `
		SELECT task_id, ticktick_task_id, project_id, sync_status, last_error
		FROM user_ticktick_task_links
		WHERE user_id=$1 AND task_id=$2`, userID, taskID).Scan(
		&link.TaskID, &link.TickTickTaskID, &link.ProjectID, &link.SyncStatus, &link.LastError,
	)
	if err == sql.ErrNoRows {
		return domain.TickTickTaskLink{}, domain.ErrNotFound
	}
	return link, err
}

func (s *Repository) SaveTickTickTaskLink(ctx context.Context, link domain.TickTickTaskLink) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		INSERT INTO user_ticktick_task_links
			(user_id, task_id, ticktick_task_id, project_id, sync_status, last_error)
		SELECT $1, task.id, $3, $4, $5, $6
		FROM tasks AS task
		WHERE task.id=$2 AND task.user_id=$1
		ON CONFLICT (task_id) DO UPDATE
		SET ticktick_task_id=EXCLUDED.ticktick_task_id,
		    project_id=EXCLUDED.project_id,
		    sync_status=EXCLUDED.sync_status,
		    last_error=EXCLUDED.last_error,
		    updated_at=now()`, userID, link.TaskID, link.TickTickTaskID, link.ProjectID, link.SyncStatus, link.LastError)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("task: %w", domain.ErrNotFound)
	}
	return nil
}

func (s *Repository) TickTickPendingTasks(ctx context.Context) ([]domain.Task, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, taskSelect+`
		WHERE task.user_id=$1 AND ticktick.sync_status IN ('pending', 'error')
		ORDER BY ticktick.updated_at, task.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Task{}
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, task)
	}
	return out, rows.Err()
}

func (s *Repository) ApplyTickTickSnapshot(ctx context.Context, remoteTasks []domain.TickTickRemoteTask) (domain.TickTickPullResult, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.TickTickPullResult{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.TickTickPullResult{}, err
	}
	defer tx.Rollback()

	result := domain.TickTickPullResult{}
	seenIDs := make([]string, 0, len(remoteTasks))
	for _, remote := range remoteTasks {
		seenIDs = append(seenIDs, remote.ID)

		var taskID int64
		var syncStatus, source string
		err := tx.QueryRowContext(ctx, `
			SELECT task_id, sync_status, source
			FROM user_ticktick_task_links
			WHERE user_id=$1 AND ticktick_task_id=$2
			FOR UPDATE`, userID, remote.ID).Scan(&taskID, &syncStatus, &source)
		if err == sql.ErrNoRows {
			err = tx.QueryRowContext(ctx, `
				INSERT INTO tasks (
					user_id, title, description, category, status,
					due_date, due_time, completed_at, priority, is_milestone
				)
				VALUES ($1, $2, $3, 'TickTick', 'todo', NULLIF($4, '')::date, NULLIF($5, '')::time, NULL, $6, $7)
				RETURNING id`, userID, remote.Title, remote.Description, remote.DueDate, remote.DueTime, remote.Priority, remote.IsMilestone).Scan(&taskID)
			if err != nil {
				return domain.TickTickPullResult{}, err
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO user_ticktick_task_links (
					user_id, task_id, ticktick_task_id, project_id, sync_status, last_error, source
				)
				VALUES ($1, $2, $3, $4, 'synced', '', 'ticktick')`,
				userID, taskID, remote.ID, remote.ProjectID); err != nil {
				return domain.TickTickPullResult{}, err
			}
			result.Imported++
			continue
		}
		if err != nil {
			return domain.TickTickPullResult{}, err
		}

		// Даже если локальная отправка временно не прошла, сохраняем новое
		// расположение удалённой задачи, чтобы следующий retry использовал
		// актуальный projectId. Сами локальные поля до успешной отправки не затираем.
		if _, err := tx.ExecContext(ctx, `
			UPDATE user_ticktick_task_links
			SET project_id=$3, updated_at=now()
			WHERE user_id=$1 AND task_id=$2`, userID, taskID, remote.ProjectID); err != nil {
			return domain.TickTickPullResult{}, err
		}
		if syncStatus != "synced" {
			continue
		}

		category := ""
		if source == "ticktick" {
			category = "TickTick"
		}
		updated, err := tx.ExecContext(ctx, `
			UPDATE tasks
			SET title=$3,
			    description=$4,
			    category=CASE WHEN $9<>'' THEN $9 ELSE category END,
			    status='todo',
			    completed_at=NULL,
			    due_date=NULLIF($5, '')::date,
			    due_time=NULLIF($6, '')::time,
			    priority=$7,
			    is_milestone=$8
			WHERE id=$1 AND user_id=$2
			  AND (
				title IS DISTINCT FROM $3 OR
				description IS DISTINCT FROM $4 OR
				($9<>'' AND category IS DISTINCT FROM $9) OR
				status IS DISTINCT FROM 'todo' OR
				due_date IS DISTINCT FROM NULLIF($5, '')::date OR
				due_time IS DISTINCT FROM NULLIF($6, '')::time OR
				priority IS DISTINCT FROM $7 OR
				is_milestone IS DISTINCT FROM $8
			  )`, taskID, userID, remote.Title, remote.Description, remote.DueDate, remote.DueTime, remote.Priority, remote.IsMilestone, category)
		if err != nil {
			return domain.TickTickPullResult{}, err
		}
		rows, err := updated.RowsAffected()
		if err != nil {
			return domain.TickTickPullResult{}, err
		}
		result.Updated += int(rows)
	}

	completedQuery := `
		UPDATE tasks AS task
		SET status='done', completed_at=COALESCE(task.completed_at, now())
		FROM user_ticktick_task_links AS link
		WHERE link.user_id=$1
		  AND link.task_id=task.id
		  AND task.user_id=$1
		  AND task.status='todo'
		  AND link.sync_status='synced'
		  AND link.ticktick_task_id<>''`
	completedArgs := []any{userID}
	if len(seenIDs) > 0 {
		placeholders := make([]string, len(seenIDs))
		for index, remoteID := range seenIDs {
			placeholders[index] = fmt.Sprintf("$%d", index+2)
			completedArgs = append(completedArgs, remoteID)
		}
		completedQuery += " AND link.ticktick_task_id NOT IN (" + strings.Join(placeholders, ",") + ")"
	}
	completed, err := tx.ExecContext(ctx, completedQuery, completedArgs...)
	if err != nil {
		return domain.TickTickPullResult{}, err
	}
	rows, err := completed.RowsAffected()
	if err != nil {
		return domain.TickTickPullResult{}, err
	}
	result.Completed = int(rows)

	if err := tx.Commit(); err != nil {
		return domain.TickTickPullResult{}, err
	}
	return result, nil
}

func (s *Repository) TickTickSyncCounts(ctx context.Context) (int, int, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return 0, 0, err
	}
	var pending, failed int
	err = s.db.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE sync_status='pending'),
		       count(*) FILTER (WHERE sync_status='error')
		FROM user_ticktick_task_links WHERE user_id=$1`, userID).Scan(&pending, &failed)
	return pending, failed, err
}
