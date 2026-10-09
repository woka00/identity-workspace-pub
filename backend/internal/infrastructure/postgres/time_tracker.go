package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"avatar-id/internal/domain"
	"github.com/lib/pq"
)

func (s *Repository) TimeTracker(ctx context.Context, trackerRange domain.TimeTrackerRange) (domain.TimeTrackerState, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.TimeTrackerState{}, err
	}
	state := domain.TimeTrackerState{
		Date:       trackerRange.Date,
		Period:     trackerRange.Period,
		Activities: []domain.TimeActivity{},
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT activity.id, activity.name, activity.sort_order, activity.created_at,
		       COALESCE(activity.reminder_interval_minutes, 0), COALESCE(activity.reminder_message, ''),
		       COALESCE((
		           SELECT floor(sum(greatest(0, extract(epoch FROM
		               least(COALESCE(session.ended_at, $6), $3) - greatest(session.started_at, $2)
		           ))))::bigint
		           FROM user_time_sessions AS session
		           WHERE session.user_id=$1 AND session.activity_id=activity.id
		             AND session.started_at < $3 AND COALESCE(session.ended_at, $6) > $2
		       ), 0),
		       COALESCE((
		           SELECT floor(sum(greatest(0, extract(epoch FROM
		               least(COALESCE(session.ended_at, $6), $5) - greatest(session.started_at, $4)
		           ))))::bigint
		           FROM user_time_sessions AS session
		           WHERE session.user_id=$1 AND session.activity_id=activity.id
		             AND session.started_at < $5 AND COALESCE(session.ended_at, $6) > $4
		       ), 0),
		       (
		           SELECT max(session.started_at)
		           FROM user_time_sessions AS session
		           WHERE session.user_id=$1 AND session.activity_id=activity.id
		       ),
		       EXISTS (
		           SELECT 1 FROM user_time_sessions AS session
		           WHERE session.user_id=$1 AND session.activity_id=activity.id AND session.ended_at IS NULL
		       )
		FROM user_time_activities AS activity
		WHERE activity.user_id=$1 AND activity.archived_at IS NULL
		ORDER BY activity.sort_order, activity.created_at, activity.id`,
		userID,
		trackerRange.TodayStart,
		trackerRange.TomorrowStart,
		trackerRange.PeriodStart,
		trackerRange.PeriodEnd,
		trackerRange.Now,
	)
	if err != nil {
		return domain.TimeTrackerState{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var activity domain.TimeActivity
		var createdAt time.Time
		var lastStartedAt sql.NullTime
		if err := rows.Scan(
			&activity.ID,
			&activity.Name,
			&activity.SortOrder,
			&createdAt,
			&activity.ReminderIntervalMinutes,
			&activity.ReminderMessage,
			&activity.TodaySeconds,
			&activity.PeriodSeconds,
			&lastStartedAt,
			&activity.Active,
		); err != nil {
			return domain.TimeTrackerState{}, err
		}
		activity.CreatedAt = createdAt.Format(time.RFC3339Nano)
		if lastStartedAt.Valid {
			activity.LastStartedDate = lastStartedAt.Time.In(trackerRange.Now.Location()).Format("2006-01-02")
		}
		state.Activities = append(state.Activities, activity)
	}
	if err := rows.Err(); err != nil {
		return domain.TimeTrackerState{}, err
	}
	if err := rows.Close(); err != nil {
		return domain.TimeTrackerState{}, err
	}
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(floor(sum(greatest(0, extract(epoch FROM
		           least(COALESCE(ended_at, $6), $3) - greatest(started_at, $2)
		       ))))::bigint, 0),
		       COALESCE(floor(sum(greatest(0, extract(epoch FROM
		           least(COALESCE(ended_at, $6), $5) - greatest(started_at, $4)
		       ))))::bigint, 0)
		FROM user_time_sessions
		WHERE user_id=$1
		  AND started_at < $5
		  AND COALESCE(ended_at, $6) > LEAST($2, $4)`,
		userID,
		trackerRange.TodayStart,
		trackerRange.TomorrowStart,
		trackerRange.PeriodStart,
		trackerRange.PeriodEnd,
		trackerRange.Now,
	).Scan(&state.TodayTotalSeconds, &state.PeriodTotalSeconds); err != nil {
		return domain.TimeTrackerState{}, err
	}

	var active domain.ActiveTimeSession
	var paused bool
	var startedAt time.Time
	err = s.db.QueryRowContext(ctx, `
		SELECT session.id, activity.id, activity.name, session.started_at,
		       floor(session.carried_seconds + greatest(0, extract(epoch FROM (COALESCE(session.ended_at, $2) - session.started_at))))::bigint,
		       session.paused
		FROM user_time_sessions AS session
		JOIN user_time_activities AS activity ON activity.id=session.activity_id AND activity.user_id=session.user_id
		WHERE session.user_id=$1 AND (session.ended_at IS NULL OR session.paused)`, userID, trackerRange.Now).Scan(
		&active.ID,
		&active.ActivityID,
		&active.ActivityName,
		&startedAt,
		&active.ElapsedSeconds,
		&paused,
	)
	if err == nil {
		active.StartedAt = startedAt.Format(time.RFC3339Nano)
		if paused {
			state.PausedSession = &active
		} else {
			state.ActiveSession = &active
		}
	} else if err != sql.ErrNoRows {
		return domain.TimeTrackerState{}, err
	}
	return state, nil
}

func (s *Repository) CreateTimeActivity(ctx context.Context, input domain.TimeActivityInput) (domain.TimeActivity, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.TimeActivity{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.TimeActivity{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, userID); err != nil {
		return domain.TimeActivity{}, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM user_time_activities WHERE user_id=$1 AND archived_at IS NULL`, userID).Scan(&count); err != nil {
		return domain.TimeActivity{}, err
	}
	if count >= 50 {
		return domain.TimeActivity{}, fmt.Errorf("time activity limit: %w", domain.ErrConflict)
	}
	var activity domain.TimeActivity
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
		INSERT INTO user_time_activities (user_id, name, sort_order, reminder_interval_minutes, reminder_message)
		SELECT $1, $2, COALESCE(max(sort_order), 0) + 1, NULLIF($3, 0), NULLIF($4, '')
		FROM user_time_activities
		WHERE user_id=$1 AND archived_at IS NULL
		ON CONFLICT DO NOTHING
		RETURNING id, name, sort_order, created_at,
		          COALESCE(reminder_interval_minutes, 0), COALESCE(reminder_message, '')`,
		userID, input.Name, input.ReminderIntervalMinutes, input.ReminderMessage).Scan(
		&activity.ID,
		&activity.Name,
		&activity.SortOrder,
		&createdAt,
		&activity.ReminderIntervalMinutes,
		&activity.ReminderMessage,
	)
	if err == sql.ErrNoRows {
		return domain.TimeActivity{}, fmt.Errorf("time activity already exists: %w", domain.ErrConflict)
	}
	if err != nil {
		return domain.TimeActivity{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.TimeActivity{}, err
	}
	activity.CreatedAt = createdAt.Format(time.RFC3339Nano)
	return activity, nil
}

func (s *Repository) UpdateTimeActivity(ctx context.Context, id int64, input domain.TimeActivityInput, now time.Time) (domain.TimeActivity, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.TimeActivity{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.TimeActivity{}, err
	}
	defer tx.Rollback()
	var previousReminderInterval int
	var previousReminderMessage string
	err = tx.QueryRowContext(ctx, `
		SELECT COALESCE(reminder_interval_minutes, 0), COALESCE(reminder_message, '')
		FROM user_time_activities
		WHERE id=$1 AND user_id=$2 AND archived_at IS NULL
		FOR UPDATE`, id, userID).Scan(&previousReminderInterval, &previousReminderMessage)
	if err == sql.ErrNoRows {
		return domain.TimeActivity{}, fmt.Errorf("time activity: %w", domain.ErrNotFound)
	}
	if err != nil {
		return domain.TimeActivity{}, err
	}
	var activity domain.TimeActivity
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `
		UPDATE user_time_activities
		SET name=$3,
		    reminder_interval_minutes=NULLIF($4, 0),
		    reminder_message=NULLIF($5, ''),
		    updated_at=now()
		WHERE id=$1 AND user_id=$2 AND archived_at IS NULL
		RETURNING id, name, sort_order, created_at,
		          COALESCE(reminder_interval_minutes, 0), COALESCE(reminder_message, '')`,
		id, userID, input.Name, input.ReminderIntervalMinutes, input.ReminderMessage).Scan(
		&activity.ID,
		&activity.Name,
		&activity.SortOrder,
		&createdAt,
		&activity.ReminderIntervalMinutes,
		&activity.ReminderMessage,
	)
	if err != nil {
		var postgresError *pq.Error
		if errors.As(err, &postgresError) && postgresError.Code == "23505" {
			return domain.TimeActivity{}, fmt.Errorf("time activity already exists: %w", domain.ErrConflict)
		}
		return domain.TimeActivity{}, err
	}
	if previousReminderInterval != input.ReminderIntervalMinutes || previousReminderMessage != input.ReminderMessage {
		if _, err := tx.ExecContext(ctx, `
			UPDATE user_time_sessions
			SET next_reminder_at=CASE
			        WHEN $4::integer > 0 THEN $3::timestamptz + ($4::integer * interval '1 minute')
			        ELSE NULL
			    END,
			    reminder_claimed_at=NULL,
			    reminder_attempted_at=NULL
			WHERE user_id=$1 AND activity_id=$2 AND ended_at IS NULL`,
			userID, id, now, input.ReminderIntervalMinutes); err != nil {
			return domain.TimeActivity{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return domain.TimeActivity{}, err
	}
	activity.CreatedAt = createdAt.Format(time.RFC3339Nano)
	return activity, nil
}

func (s *Repository) DeleteTimeActivity(ctx context.Context, id int64, now time.Time) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, userID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `
		UPDATE user_time_activities
		SET archived_at=$3, updated_at=now()
		WHERE id=$1 AND user_id=$2 AND archived_at IS NULL`, id, userID, now)
	if err != nil {
		return err
	}
	archived, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if archived == 0 {
		return fmt.Errorf("time activity: %w", domain.ErrNotFound)
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE user_time_sessions
		SET ended_at=COALESCE(ended_at, greatest(started_at, $3)),
		    paused=FALSE,
		    next_reminder_at=NULL,
		    reminder_claimed_at=NULL,
		    reminder_attempted_at=NULL
		WHERE user_id=$1 AND activity_id=$2 AND (ended_at IS NULL OR paused)`, userID, id, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Repository) StartTimeActivity(ctx context.Context, id int64, now time.Time) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, userID); err != nil {
		return err
	}
	var reminderInterval sql.NullInt64
	if err := tx.QueryRowContext(ctx, `
		SELECT reminder_interval_minutes
		FROM user_time_activities
		WHERE id=$1 AND user_id=$2 AND archived_at IS NULL`, id, userID).Scan(&reminderInterval); err == sql.ErrNoRows {
		return fmt.Errorf("time activity: %w", domain.ErrNotFound)
	} else if err != nil {
		return err
	}
	var activeActivityID int64
	var paused bool
	var carriedSeconds string
	err = tx.QueryRowContext(ctx, `
		SELECT activity_id, paused,
		       (carried_seconds + greatest(0, extract(epoch FROM (ended_at - started_at))))::text
		FROM user_time_sessions
		WHERE user_id=$1 AND (ended_at IS NULL OR paused)
		FOR UPDATE`, userID).Scan(&activeActivityID, &paused, &carriedSeconds)
	if err == nil && activeActivityID == id && !paused {
		return tx.Commit()
	}
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE user_time_sessions
		SET ended_at=COALESCE(ended_at, greatest(started_at, $2)), paused=FALSE,
		    next_reminder_at=NULL, reminder_claimed_at=NULL, reminder_attempted_at=NULL
		WHERE user_id=$1 AND (ended_at IS NULL OR paused)`, userID, now); err != nil {
		return err
	}
	if !paused || activeActivityID != id {
		carriedSeconds = "0"
	}
	var nextReminderAt any
	if reminderInterval.Valid {
		nextReminderAt = now.Add(time.Duration(reminderInterval.Int64) * time.Minute)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_time_sessions (user_id, activity_id, started_at, next_reminder_at, carried_seconds)
		VALUES ($1, $2, $3, $4, $5)`, userID, id, now, nextReminderAt, carriedSeconds); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Repository) ClaimDueTimeActivityReminders(ctx context.Context, now time.Time, limit int) ([]domain.TimeActivityReminderJob, error) {
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
			SELECT session.id
			FROM user_time_sessions AS session
			JOIN user_time_activities AS activity
			  ON activity.id=session.activity_id AND activity.user_id=session.user_id
			WHERE session.ended_at IS NULL
			  AND activity.archived_at IS NULL
			  AND activity.reminder_interval_minutes IS NOT NULL
			  AND session.next_reminder_at <= $1
			  AND (session.reminder_claimed_at IS NULL OR session.reminder_claimed_at < $1 - interval '2 minutes')
			  AND (session.reminder_attempted_at IS NULL OR session.reminder_attempted_at < $1 - interval '5 minutes')
			ORDER BY session.next_reminder_at, session.id
			LIMIT $2
			FOR UPDATE OF session SKIP LOCKED
		), claimed AS (
			UPDATE user_time_sessions AS session
			SET reminder_claimed_at=$1, reminder_attempted_at=$1
			FROM due
			WHERE session.id=due.id
			RETURNING session.id, session.user_id, session.activity_id
		)
		SELECT claimed.id, claimed.activity_id, activity.name,
		       COALESCE(activity.reminder_message, 'Пора сделать перерыв и немного подвигаться'),
		       claimed.user_id
		FROM claimed
		JOIN user_time_activities AS activity
		  ON activity.id=claimed.activity_id AND activity.user_id=claimed.user_id`, now.UTC(), limit)
	if err != nil {
		return nil, err
	}
	type claimedJob struct {
		domain.TimeActivityReminderJob
		userID int64
	}
	claimed := []claimedJob{}
	for rows.Next() {
		var job claimedJob
		if err := rows.Scan(&job.SessionID, &job.ActivityID, &job.ActivityName, &job.Message, &job.userID); err != nil {
			rows.Close()
			return nil, err
		}
		claimed = append(claimed, job)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	jobs := make([]domain.TimeActivityReminderJob, 0, len(claimed))
	for _, claimedJob := range claimed {
		job := claimedJob.TimeActivityReminderJob
		subscriptions, err := s.pushSubscriptionsForUser(ctx, claimedJob.userID)
		if err != nil {
			return nil, err
		}
		job.Subscriptions = subscriptions
		jobs = append(jobs, job)
	}
	return jobs, nil
}

func (s *Repository) CompleteTimeActivityReminder(ctx context.Context, sessionID int64, now time.Time, sent bool) error {
	if sent {
		_, err := s.db.ExecContext(ctx, `
			UPDATE user_time_sessions AS session
			SET next_reminder_at=$2::timestamptz + activity.reminder_interval_minutes * interval '1 minute',
			    reminder_claimed_at=NULL
			FROM user_time_activities AS activity
			WHERE session.id=$1
			  AND session.ended_at IS NULL
			  AND activity.id=session.activity_id
			  AND activity.user_id=session.user_id
			  AND activity.reminder_interval_minutes IS NOT NULL`, sessionID, now.UTC())
		return err
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE user_time_sessions
		SET reminder_claimed_at=NULL
		WHERE id=$1`, sessionID)
	return err
}

func (s *Repository) StopTimeActivity(ctx context.Context, now time.Time) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE user_time_sessions
		SET ended_at=COALESCE(ended_at, greatest(started_at, $2)), paused=FALSE,
		    next_reminder_at=NULL, reminder_claimed_at=NULL, reminder_attempted_at=NULL
		WHERE user_id=$1 AND (ended_at IS NULL OR paused)`, userID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Repository) PauseTimeActivity(ctx context.Context, now time.Time) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE user_time_sessions
		SET ended_at=greatest(started_at, $2), paused=TRUE,
		    next_reminder_at=NULL, reminder_claimed_at=NULL, reminder_attempted_at=NULL
		WHERE user_id=$1 AND ended_at IS NULL`, userID, now); err != nil {
		return err
	}
	return tx.Commit()
}
