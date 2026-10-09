package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"avatar-id/internal/domain"
)

func (s *Repository) GitHubTracker(ctx context.Context) (domain.GitHubTrackerRecord, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.GitHubTrackerRecord{}, err
	}
	var record domain.GitHubTrackerRecord
	var activityJSON []byte
	err = s.db.QueryRowContext(ctx, `
		SELECT github_username, cached_activity,
		       COALESCE(to_char(activity_fetched_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), '')
		FROM user_github_tracker_settings
		WHERE user_id=$1`, userID).Scan(&record.Username, &activityJSON, &record.FetchedAt)
	if err == sql.ErrNoRows {
		return domain.GitHubTrackerRecord{}, fmt.Errorf("github tracker: %w", domain.ErrNotFound)
	}
	if err != nil {
		return domain.GitHubTrackerRecord{}, err
	}
	if len(activityJSON) > 0 && string(activityJSON) != "{}" {
		if err := json.Unmarshal(activityJSON, &record.Activity); err != nil {
			return domain.GitHubTrackerRecord{}, fmt.Errorf("decode github tracker cache: %w", err)
		}
	}
	return record, nil
}

func (s *Repository) UpsertGitHubTracker(ctx context.Context, username string) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO user_github_tracker_settings (user_id, github_username)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE
		SET github_username=EXCLUDED.github_username,
		    cached_activity=CASE
		      WHEN user_github_tracker_settings.github_username=EXCLUDED.github_username
		      THEN user_github_tracker_settings.cached_activity
		      ELSE '{}'::jsonb
		    END,
		    activity_fetched_at=CASE
		      WHEN user_github_tracker_settings.github_username=EXCLUDED.github_username
		      THEN user_github_tracker_settings.activity_fetched_at
		      ELSE NULL
		    END,
		    updated_at=now()`, userID, username)
	return err
}

func (s *Repository) CacheGitHubTracker(ctx context.Context, username string, activity domain.GitHubTrackerState) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(activity)
	if err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE user_github_tracker_settings
		SET cached_activity=$3::jsonb, activity_fetched_at=now(), updated_at=now()
		WHERE user_id=$1 AND github_username=$2`, userID, username, payload)
	if err != nil {
		return err
	}
	if count, _ := result.RowsAffected(); count == 0 {
		return fmt.Errorf("github tracker: %w", domain.ErrNotFound)
	}
	return nil
}

func (s *Repository) DeleteGitHubTracker(ctx context.Context) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM user_github_tracker_settings WHERE user_id=$1`, userID)
	return err
}
