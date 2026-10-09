package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"avatar-id/internal/application"
	"avatar-id/internal/domain"
)

func TestUpdateExistingTimeActivitySchedulesReminder(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_TEST_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_TEST_URL is not set")
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	repository := New(db)
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var userID int64
	if err := db.QueryRowContext(ctx, `SELECT id FROM users ORDER BY id LIMIT 1`).Scan(&userID); err != nil {
		t.Fatal(err)
	}

	name := fmt.Sprintf("existing-timer-reminder-%d", time.Now().UnixNano())
	var activityID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO user_time_activities (user_id, name, sort_order)
		VALUES ($1,$2,9999)
		RETURNING id`, userID, name).Scan(&activityID); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = db.Exec(`DELETE FROM user_time_activities WHERE id=$1`, activityID) }()

	now := time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
	var sessionID int64
	if err := db.QueryRowContext(ctx, `
		INSERT INTO user_time_sessions (user_id, activity_id, started_at)
		VALUES ($1,$2,$3)
		RETURNING id`, userID, activityID, now.Add(-10*time.Minute)).Scan(&sessionID); err != nil {
		t.Fatal(err)
	}

	userContext := application.WithUser(ctx, domain.User{ID: userID})
	updated, err := repository.UpdateTimeActivity(userContext, activityID, domain.TimeActivityInput{
		Name:                    name,
		ReminderIntervalMinutes: 30,
		ReminderMessage:         "Пора размяться",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ReminderIntervalMinutes != 30 || updated.ReminderMessage != "Пора размяться" {
		t.Fatalf("updated activity = %+v", updated)
	}

	var nextReminderAt time.Time
	if err := db.QueryRowContext(ctx, `SELECT next_reminder_at FROM user_time_sessions WHERE id=$1`, sessionID).Scan(&nextReminderAt); err != nil {
		t.Fatal(err)
	}
	if want := now.Add(30 * time.Minute); !nextReminderAt.Equal(want) {
		t.Fatalf("next reminder = %s, want %s", nextReminderAt, want)
	}

	secondDeliveryAt := now.Add(30 * time.Minute)
	if err := repository.CompleteTimeActivityReminder(ctx, sessionID, secondDeliveryAt, true); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx, `SELECT next_reminder_at FROM user_time_sessions WHERE id=$1`, sessionID).Scan(&nextReminderAt); err != nil {
		t.Fatal(err)
	}
	if want := secondDeliveryAt.Add(30 * time.Minute); !nextReminderAt.Equal(want) {
		t.Fatalf("repeated reminder = %s, want %s", nextReminderAt, want)
	}
}

func TestTimeTrackerPauseResumeLifecycle(t *testing.T) {
	databaseURL := os.Getenv("DATABASE_TEST_URL")
	if databaseURL == "" {
		t.Skip("DATABASE_TEST_URL is not set")
	}
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	repository := New(db)
	if err := repository.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	newUser := func() context.Context {
		login := fmt.Sprintf("timer-pause-test-%d", time.Now().UnixNano())
		var id int64
		if err := db.QueryRowContext(ctx, `INSERT INTO users (login, login_normalized, password_hash) VALUES ($1,$1,'test') RETURNING id`, login).Scan(&id); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = db.Exec(`DELETE FROM users WHERE id=$1`, id) })
		return application.WithUser(ctx, domain.User{ID: id})
	}
	userCtx, otherCtx := newUser(), newUser()
	now := time.Date(2026, time.September, 10, 23, 59, 30, 0, time.UTC)
	service := application.New(repository, nil, func() time.Time { return now })
	activity, err := service.CreateTimeActivity(userCtx, domain.TimeActivityInput{Name: "Work", ReminderIntervalMinutes: 5})
	if err != nil {
		t.Fatal(err)
	}
	other, err := service.CreateTimeActivity(userCtx, domain.TimeActivityInput{Name: "Read"})
	if err != nil {
		t.Fatal(err)
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err = service.StartTimeActivity(userCtx, activity.ID, "week")
	must(err)
	now = now.Add(30 * time.Second)
	state, err := service.PauseTimeActivity(userCtx, "week")
	must(err)
	if state.ActiveSession != nil || state.PausedSession == nil || state.PausedSession.ElapsedSeconds != 30 {
		t.Fatalf("pause = %+v", state)
	}
	var pending int
	must(db.QueryRowContext(ctx, `SELECT count(*) FROM user_time_sessions WHERE activity_id=$1 AND next_reminder_at IS NOT NULL`, activity.ID).Scan(&pending))
	if pending != 0 {
		t.Fatal("paused timer has pending reminders")
	}
	now = now.Add(time.Hour)
	_, err = service.PauseTimeActivity(userCtx, "week")
	must(err)
	_, err = service.StopTimeActivity(otherCtx, "week")
	must(err)
	// A fresh repository must recover the frozen timer; the pause spans midnight.
	service = application.New(New(db), nil, func() time.Time { return now })
	state, err = service.TimeTracker(userCtx, "week")
	must(err)
	if state.PausedSession == nil || state.PausedSession.ElapsedSeconds != 30 || state.TodayTotalSeconds != 0 || state.PeriodTotalSeconds != 30 {
		t.Fatalf("paused refresh = %+v", state)
	}
	if _, err := service.StartTimeActivity(otherCtx, activity.ID, "week"); err == nil {
		t.Fatal("another user started this timer")
	}
	state, err = service.StartTimeActivity(userCtx, activity.ID, "week")
	must(err)
	if state.PausedSession != nil || state.ActiveSession == nil || state.ActiveSession.ElapsedSeconds != 30 {
		t.Fatalf("resume = %+v", state)
	}
	now = now.Add(20 * time.Second)
	_, err = service.StartTimeActivity(userCtx, activity.ID, "week")
	must(err)
	state, err = service.PauseTimeActivity(userCtx, "week")
	must(err)
	if state.PausedSession == nil || state.PausedSession.ElapsedSeconds != 50 || state.TodayTotalSeconds != 20 || state.PeriodTotalSeconds != 50 {
		t.Fatalf("second pause = %+v", state)
	}
	now = now.Add(time.Hour)
	state, err = service.StopTimeActivity(userCtx, "week")
	must(err)
	if state.ActiveSession != nil || state.PausedSession != nil || state.PeriodTotalSeconds != 50 {
		t.Fatalf("finish paused = %+v", state)
	}
	state, err = service.StartTimeActivity(userCtx, activity.ID, "week")
	must(err)
	if state.ActiveSession.ElapsedSeconds != 0 {
		t.Fatal("finished timer resumed old stopwatch")
	}
	// Switching from a paused timer selects only the new timer.
	now = now.Add(10 * time.Second)
	_, err = service.PauseTimeActivity(userCtx, "week")
	must(err)
	state, err = service.StartTimeActivity(userCtx, other.ID, "week")
	must(err)
	if state.PausedSession != nil || state.ActiveSession.ActivityID != other.ID || state.ActiveSession.ElapsedSeconds != 0 {
		t.Fatalf("switch = %+v", state)
	}
	_, err = service.PauseTimeActivity(userCtx, "week")
	must(err)
	must(service.DeleteTimeActivity(userCtx, other.ID))
	state, err = service.TimeTracker(userCtx, "week")
	must(err)
	if state.PausedSession != nil || state.ActiveSession != nil || state.PeriodTotalSeconds != 60 {
		t.Fatalf("delete paused = %+v", state)
	}
}
