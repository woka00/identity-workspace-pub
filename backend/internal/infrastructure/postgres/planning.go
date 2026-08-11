package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"identity-workspace/internal/domain"
)

func (s *Repository) TaskCategories(ctx context.Context) ([]domain.TaskCategory, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, lower(name) IN ('дом', 'работа') AS builtin FROM user_task_categories
		WHERE user_id=$1 ORDER BY CASE lower(name) WHEN 'дом' THEN 0 WHEN 'работа' THEN 1 ELSE 2 END, name`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.TaskCategory{}
	for rows.Next() {
		var category domain.TaskCategory
		if err := rows.Scan(&category.ID, &category.Name, &category.Builtin); err != nil {
			return nil, err
		}
		out = append(out, category)
	}
	return out, rows.Err()
}

func (s *Repository) CreateTaskCategory(ctx context.Context, name string) (domain.TaskCategory, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.TaskCategory{}, err
	}
	var category domain.TaskCategory
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO user_task_categories (user_id, name, name_normalized)
		VALUES ($1, $2, lower($2))
		ON CONFLICT (user_id, name_normalized) DO UPDATE SET name=EXCLUDED.name
		RETURNING id, name, lower(name) IN ('дом', 'работа') AS builtin`, userID, name).Scan(&category.ID, &category.Name, &category.Builtin)
	return category, err
}

func (s *Repository) DeleteTaskCategory(ctx context.Context, id int64) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var name string
	if err := tx.QueryRowContext(ctx, `
		SELECT name FROM user_task_categories
		WHERE id=$1 AND user_id=$2
		FOR UPDATE`, id, userID).Scan(&name); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("task category: %w", domain.ErrNotFound)
		}
		return err
	}
	if normalized := strings.ToLower(strings.TrimSpace(name)); normalized == "дом" || normalized == "работа" {
		return fmt.Errorf("категории «Дом» и «Работа» удалить нельзя: %w", domain.ErrConflict)
	}

	var activeCount int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*) FROM tasks
		WHERE user_id=$1 AND status <> 'done' AND lower(btrim(category))=lower(btrim($2))`, userID, name).Scan(&activeCount); err != nil {
		return err
	}
	if activeCount > 0 {
		return fmt.Errorf("в категории «%s» есть активные задачи (%d); сначала завершите или перенесите их: %w", name, activeCount, domain.ErrConflict)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM user_task_categories WHERE id=$1 AND user_id=$2`, id, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func scanCustomTracker(row interface{ Scan(...any) error }) (domain.CustomTracker, error) {
	var tracker domain.CustomTracker
	err := row.Scan(&tracker.ID, &tracker.Name, &tracker.TargetValue, &tracker.StepValue,
		&tracker.CurrentValue, &tracker.Icon, &tracker.CreatedAt, &tracker.UpdatedAt)
	return tracker, err
}

const customTrackerSelect = `
	SELECT id, name, target_value::double precision, step_value::double precision,
	       current_value::double precision, icon,
	       to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
	       to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')
	FROM user_custom_trackers`

func (s *Repository) CustomTrackers(ctx context.Context) ([]domain.CustomTracker, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, customTrackerSelect+` WHERE user_id=$1 ORDER BY created_at, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.CustomTracker{}
	for rows.Next() {
		tracker, err := scanCustomTracker(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, tracker)
	}
	return out, rows.Err()
}

func (s *Repository) CreateCustomTracker(ctx context.Context, in domain.CustomTrackerInput) (domain.CustomTracker, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.CustomTracker{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.CustomTracker{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, userID); err != nil {
		return domain.CustomTracker{}, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM user_custom_trackers WHERE user_id=$1`, userID).Scan(&count); err != nil {
		return domain.CustomTracker{}, err
	}
	if count >= 20 {
		return domain.CustomTracker{}, fmt.Errorf("custom tracker limit: %w", domain.ErrConflict)
	}
	tracker, err := scanCustomTracker(tx.QueryRowContext(ctx, `
		INSERT INTO user_custom_trackers (user_id, name, target_value, step_value, icon)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, name, target_value::double precision, step_value::double precision,
		          current_value::double precision, icon,
		          to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		          to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')`,
		userID, in.Name, in.TargetValue, in.StepValue, in.Icon,
	))
	if err != nil {
		return domain.CustomTracker{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.CustomTracker{}, err
	}
	return tracker, nil
}

func (s *Repository) UpdateCustomTracker(ctx context.Context, id int64, in domain.CustomTrackerInput) (domain.CustomTracker, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.CustomTracker{}, err
	}
	tracker, err := scanCustomTracker(s.db.QueryRowContext(ctx, `
		UPDATE user_custom_trackers
		SET name=$3, target_value=$4, step_value=$5, icon=$6,
		    current_value=LEAST(current_value, $4), updated_at=now()
		WHERE id=$1 AND user_id=$2
		RETURNING id, name, target_value::double precision, step_value::double precision,
		          current_value::double precision, icon,
		          to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		          to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')`,
		id, userID, in.Name, in.TargetValue, in.StepValue, in.Icon,
	))
	if err == sql.ErrNoRows {
		return domain.CustomTracker{}, fmt.Errorf("custom tracker: %w", domain.ErrNotFound)
	}
	return tracker, err
}

func (s *Repository) StepCustomTracker(ctx context.Context, id int64, date string, direction int) (domain.CustomTracker, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.CustomTracker{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.CustomTracker{}, err
	}
	defer tx.Rollback()
	tracker, err := scanCustomTracker(tx.QueryRowContext(ctx, `
		UPDATE user_custom_trackers
		SET current_value=GREATEST(0, LEAST(target_value, current_value + step_value * $3)), updated_at=now()
		WHERE id=$1 AND user_id=$2
		RETURNING id, name, target_value::double precision, step_value::double precision,
		          current_value::double precision, icon,
		          to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		          to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')`, id, userID, direction))
	if err == sql.ErrNoRows {
		return domain.CustomTracker{}, fmt.Errorf("custom tracker: %w", domain.ErrNotFound)
	}
	if err != nil {
		return domain.CustomTracker{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_custom_tracker_entries (user_id, tracker_id, tracked_on, value, target_value)
		VALUES ($1, $2, $3::date, $4, $5)
		ON CONFLICT (user_id, tracker_id, tracked_on) DO UPDATE
		SET value=EXCLUDED.value, target_value=EXCLUDED.target_value, updated_at=now()`,
		userID, tracker.ID, date, tracker.CurrentValue, tracker.TargetValue); err != nil {
		return domain.CustomTracker{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.CustomTracker{}, err
	}
	return tracker, nil
}

func (s *Repository) DeleteCustomTracker(ctx context.Context, id int64) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_tracker_reminders WHERE user_id=$1 AND tracker_key=$2`, userID, fmt.Sprintf("custom:%d", id)); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `DELETE FROM user_custom_trackers WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return fmt.Errorf("custom tracker: %w", domain.ErrNotFound)
	}
	return tx.Commit()
}

func (s *Repository) TrackerReminders(ctx context.Context) ([]domain.TrackerReminder, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT tracker_key, to_char(remind_time, 'HH24:MI'), enabled
		FROM user_tracker_reminders
		WHERE user_id=$1
		ORDER BY remind_time, tracker_key`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.TrackerReminder{}
	for rows.Next() {
		var reminder domain.TrackerReminder
		if err := rows.Scan(&reminder.TrackerKey, &reminder.Time, &reminder.Enabled); err != nil {
			return nil, err
		}
		out = append(out, reminder)
	}
	return out, rows.Err()
}

func (s *Repository) UpsertTrackerReminder(ctx context.Context, in domain.TrackerReminderInput) (domain.TrackerReminder, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.TrackerReminder{}, err
	}
	var out domain.TrackerReminder
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO user_tracker_reminders (user_id, tracker_key, remind_time, enabled)
		VALUES ($1, $2, $3::time, $4)
		ON CONFLICT (user_id, tracker_key) DO UPDATE
		SET remind_time=EXCLUDED.remind_time,
		    enabled=EXCLUDED.enabled,
		    last_sent_on=CASE
		        WHEN user_tracker_reminders.remind_time IS DISTINCT FROM EXCLUDED.remind_time
		          OR user_tracker_reminders.enabled IS DISTINCT FROM EXCLUDED.enabled
		        THEN NULL ELSE user_tracker_reminders.last_sent_on END,
		    reminder_claimed_at=NULL,
		    reminder_attempted_at=NULL,
		    updated_at=now()
		RETURNING tracker_key, to_char(remind_time, 'HH24:MI'), enabled`,
		userID, in.TrackerKey, in.Time, in.Enabled).Scan(&out.TrackerKey, &out.Time, &out.Enabled)
	return out, err
}

func (s *Repository) DeleteTrackerReminder(ctx context.Context, trackerKey string) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM user_tracker_reminders WHERE user_id=$1 AND tracker_key=$2`, userID, trackerKey)
	return err
}

func (s *Repository) ClaimDueTrackerReminders(ctx context.Context, localDate, localTime string, now time.Time, limit int) ([]domain.TrackerReminderJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		WITH due AS (
			SELECT reminder.user_id, reminder.tracker_key
			FROM user_tracker_reminders AS reminder
			WHERE reminder.enabled
			  AND reminder.remind_time <= $2::time
			  AND (reminder.last_sent_on IS NULL OR reminder.last_sent_on < $1::date)
			  AND (reminder.reminder_claimed_at IS NULL OR reminder.reminder_claimed_at < $3 - interval '2 minutes')
			  AND (reminder.reminder_attempted_at IS NULL OR reminder.reminder_attempted_at < $3 - interval '5 minutes')
			  AND (
			      reminder.tracker_key IN ('calories', 'water', 'weight')
			      OR EXISTS (
			          SELECT 1 FROM user_custom_trackers custom
			          WHERE custom.user_id=reminder.user_id
			            AND reminder.tracker_key='custom:' || custom.id::text
			      )
			  )
			ORDER BY reminder.remind_time, reminder.user_id, reminder.tracker_key
			LIMIT $4
			FOR UPDATE SKIP LOCKED
		), claimed AS (
			UPDATE user_tracker_reminders AS reminder
			SET reminder_claimed_at=$3, reminder_attempted_at=$3, updated_at=now()
			FROM due
			WHERE reminder.user_id=due.user_id AND reminder.tracker_key=due.tracker_key
			RETURNING reminder.user_id, reminder.tracker_key, reminder.remind_time
		)
		SELECT claimed.user_id,
		       claimed.tracker_key,
		       CASE claimed.tracker_key
		         WHEN 'calories' THEN 'Калории'
		         WHEN 'water' THEN 'Вода'
		         WHEN 'weight' THEN 'Вес'
		         ELSE COALESCE(custom.name, 'Трекер')
		       END,
		       to_char(claimed.remind_time, 'HH24:MI')
		FROM claimed
		LEFT JOIN user_custom_trackers custom
		  ON custom.user_id=claimed.user_id
		 AND claimed.tracker_key='custom:' || custom.id::text`, localDate, localTime, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	jobs := []domain.TrackerReminderJob{}
	for rows.Next() {
		var job domain.TrackerReminderJob
		job.LocalDate = localDate
		if err := rows.Scan(&job.UserID, &job.TrackerKey, &job.Title, &job.Time); err != nil {
			rows.Close()
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for index := range jobs {
		subs, err := s.pushSubscriptionsForUser(ctx, jobs[index].UserID)
		if err != nil {
			return nil, err
		}
		jobs[index].Subscriptions = subs
	}
	return jobs, nil
}

func (s *Repository) CompleteTrackerReminder(ctx context.Context, userID int64, trackerKey, localDate string, sent bool) error {
	if sent {
		_, err := s.db.ExecContext(ctx, `
			UPDATE user_tracker_reminders
			SET last_sent_on=$3::date, reminder_claimed_at=NULL, updated_at=now()
			WHERE user_id=$1 AND tracker_key=$2`, userID, trackerKey, localDate)
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_tracker_reminders
		SET reminder_claimed_at=NULL, updated_at=now()
		WHERE user_id=$1 AND tracker_key=$2`, userID, trackerKey)
	return err
}

func (s *Repository) SavePushSubscription(ctx context.Context, in domain.PushSubscriptionInput, userAgent string) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO user_push_subscriptions (user_id, endpoint, p256dh, auth, user_agent)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (endpoint) DO UPDATE
		SET user_id=EXCLUDED.user_id, p256dh=EXCLUDED.p256dh, auth=EXCLUDED.auth,
		    user_agent=EXCLUDED.user_agent, updated_at=now()`,
		userID, in.Endpoint, in.P256DH, in.Auth, userAgent)
	return err
}

func (s *Repository) DeletePushSubscription(ctx context.Context, endpoint string) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM user_push_subscriptions WHERE user_id=$1 AND endpoint=$2`, userID, endpoint)
	return err
}

func (s *Repository) ClaimDueReminders(ctx context.Context, now time.Time, limit int) ([]domain.ReminderJob, error) {
	if limit <= 0 || limit > 100 {
		limit = 25
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `
		WITH due AS (
			SELECT id FROM tasks
			WHERE status='todo'
			  AND reminder_at IS NOT NULL
			  AND reminder_sent_at IS NULL
			  AND reminder_at <= $1
			  AND reminder_at >= $1 - interval '24 hours'
			  AND (reminder_claimed_at IS NULL OR reminder_claimed_at < $1 - interval '2 minutes')
			  AND (reminder_attempted_at IS NULL OR reminder_attempted_at < $1 - interval '5 minutes')
			ORDER BY reminder_at, id
			LIMIT $2
			FOR UPDATE SKIP LOCKED
		)
		UPDATE tasks AS task
		SET reminder_claimed_at=$1, reminder_attempted_at=$1
		FROM due
		WHERE task.id=due.id
		RETURNING task.id, task.user_id, task.title, task.description,
		          COALESCE(to_char(task.due_date, 'YYYY-MM-DD'), ''),
		          COALESCE(to_char(task.due_time, 'HH24:MI'), ''),
		          to_char(task.reminder_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"')`, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	jobs := []domain.ReminderJob{}
	for rows.Next() {
		var job domain.ReminderJob
		if err := rows.Scan(&job.TaskID, &job.UserID, &job.Title, &job.Description, &job.DueDate, &job.DueTime, &job.ReminderAt); err != nil {
			rows.Close()
			return nil, err
		}
		jobs = append(jobs, job)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for index := range jobs {
		subs, err := s.pushSubscriptionsForUser(ctx, jobs[index].UserID)
		if err != nil {
			return nil, err
		}
		jobs[index].Subscriptions = subs
	}
	return jobs, nil
}

func (s *Repository) pushSubscriptionsForUser(ctx context.Context, userID int64) ([]domain.PushSubscription, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, endpoint, p256dh, auth FROM user_push_subscriptions WHERE user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.PushSubscription{}
	for rows.Next() {
		var subscription domain.PushSubscription
		if err := rows.Scan(&subscription.ID, &subscription.UserID, &subscription.Endpoint, &subscription.P256DH, &subscription.Auth); err != nil {
			return nil, err
		}
		out = append(out, subscription)
	}
	return out, rows.Err()
}

func (s *Repository) CompleteReminder(ctx context.Context, taskID int64, sent bool) error {
	if sent {
		_, err := s.db.ExecContext(ctx, `UPDATE tasks SET reminder_sent_at=now(), reminder_claimed_at=NULL WHERE id=$1`, taskID)
		return err
	}
	_, err := s.db.ExecContext(ctx, `UPDATE tasks SET reminder_claimed_at=NULL WHERE id=$1`, taskID)
	return err
}

func (s *Repository) DeletePushSubscriptionByID(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM user_push_subscriptions WHERE id=$1`, id)
	return err
}
