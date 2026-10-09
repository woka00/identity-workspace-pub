package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"avatar-id/internal/domain"
)

func (s *Repository) WorkspacePreferences(ctx context.Context) (domain.WorkspacePreferences, error) {
	var preferences domain.WorkspacePreferences
	userID, err := currentUserID(ctx)
	if err != nil {
		return preferences, err
	}
	var raw []byte
	if err := s.db.QueryRowContext(ctx, `SELECT workspace_preferences FROM user_profiles WHERE user_id=$1`, userID).Scan(&raw); err != nil {
		return preferences, err
	}
	err = json.Unmarshal(raw, &preferences)
	return preferences, err
}

func (s *Repository) UpdateWorkspacePreferences(ctx context.Context, preferences domain.WorkspacePreferences) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(preferences)
	if err != nil {
		return err
	}
	// Once read, the timer introduction stays read even if another device saves an older selection.
	result, err := s.db.ExecContext(ctx, `UPDATE user_profiles
		SET workspace_preferences=$2::jsonb || jsonb_build_object('timerIntroSeen',
			COALESCE((workspace_preferences->>'timerIntroSeen')::boolean, false) OR ($2::jsonb->>'timerIntroSeen')::boolean)
		WHERE user_id=$1`, userID, string(raw))
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("profile: %w", domain.ErrNotFound)
	}
	return nil
}
