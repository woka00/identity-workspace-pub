package postgres

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"testing"
	"time"

	"avatar-id/internal/application"
	"avatar-id/internal/domain"
)

func TestWorkspacePreferencesPersistenceAndUserScoping(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_TEST_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_TEST_URL is not set")
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// A connection-local table lets us exercise the migration without touching account data.
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := db.ExecContext(ctx, `
		CREATE TEMP TABLE user_profiles (
			user_id BIGINT PRIMARY KEY,
			bottom_navigation JSONB NOT NULL DEFAULT '["card", "tasks", "tracker", "profile"]'::jsonb,
			CONSTRAINT user_profiles_bottom_navigation_four_items CHECK (jsonb_array_length(bottom_navigation) = 4)
		);
		INSERT INTO user_profiles (user_id) VALUES (1), (2)`); err != nil {
		t.Fatal(err)
	}
	migration, err := migrationsFS.ReadFile("migrations/041_workspace_preferences.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	migration, err = migrationsFS.ReadFile("migrations/042_flexible_bottom_navigation.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE user_profiles SET bottom_navigation='["card","profile"]'::jsonb WHERE user_id=1`); err != nil {
		t.Fatalf("flexible navigation rejected: %v", err)
	}
	r := New(db)
	first := application.WithUser(ctx, domain.User{ID: 1})
	second := application.WithUser(ctx, domain.User{ID: 2})
	initial, err := r.WorkspacePreferences(first)
	if err != nil {
		t.Fatal(err)
	}
	if initial.OnboardingCompleted || initial.TimerIntroSeen || len(initial.Features) != 5 {
		t.Fatalf("unexpected migration defaults: %+v", initial)
	}
	preferences := domain.WorkspacePreferences{Features: []string{"tracker"}, OnboardingCompleted: true, TimerIntroSeen: true}
	if err := r.UpdateWorkspacePreferences(first, preferences); err != nil {
		t.Fatal(err)
	}
	saved, err := r.WorkspacePreferences(first)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(saved, preferences) {
		t.Fatalf("got %+v, want %+v", saved, preferences)
	}
	untouched, err := r.WorkspacePreferences(second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(untouched, initial) {
		t.Fatalf("another user's preferences changed: %+v", untouched)
	}
	preferences.TimerIntroSeen = false
	preferences.Features = []string{"widgets"}
	if err := r.UpdateWorkspacePreferences(first, preferences); err != nil {
		t.Fatal(err)
	}
	saved, err = r.WorkspacePreferences(first)
	if err != nil {
		t.Fatal(err)
	}
	if !saved.TimerIntroSeen || !reflect.DeepEqual(saved.Features, preferences.Features) {
		t.Fatalf("stale save lost the seen flag: %+v", saved)
	}
	if _, err := r.WorkspacePreferences(ctx); err == nil {
		t.Fatal("read without identity succeeded")
	}
	if err := r.UpdateWorkspacePreferences(ctx, preferences); err == nil {
		t.Fatal("write without identity succeeded")
	}
}
