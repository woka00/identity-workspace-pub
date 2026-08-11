package postgres

// Repository implements application ports using PostgreSQL.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"sort"
	"strconv"

	"identity-workspace/internal/domain"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

type Repository struct {
	db           *sql.DB
	secretCipher *SecretCipher
}

func New(db *sql.DB, ciphers ...*SecretCipher) *Repository {
	var secretCipher *SecretCipher
	if len(ciphers) > 0 {
		secretCipher = ciphers[0]
	}
	return &Repository{db: db, secretCipher: secretCipher}
}

func (s *Repository) Migrate(ctx context.Context) error {
	// A dedicated connection is required because PostgreSQL advisory locks are
	// connection-scoped. This prevents two replicas from applying the same
	// migration concurrently during a rolling deployment.
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("acquire migration connection: %w", err)
	}
	defer conn.Close()

	const migrationLockID int64 = 0x4944454E54495459 // "IDENTITY"
	if _, err := conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, migrationLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, migrationLockID)
	}()

	if _, err := conn.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			name TEXT PRIMARY KEY,
			checksum CHAR(64),
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
			);
		ALTER TABLE schema_migrations
			ADD COLUMN IF NOT EXISTS checksum CHAR(64)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		checksum := hex.EncodeToString(sum[:])

		var storedChecksum sql.NullString
		err = conn.QueryRowContext(ctx,
			`SELECT checksum FROM schema_migrations WHERE name=$1`, entry.Name(),
		).Scan(&storedChecksum)
		if err == nil {
			if storedChecksum.Valid && storedChecksum.String != "" && storedChecksum.String != checksum {
				return fmt.Errorf("migration %s checksum mismatch: an applied migration file was modified", entry.Name())
			}
			if !storedChecksum.Valid || storedChecksum.String == "" {
				if _, err := conn.ExecContext(ctx,
					`UPDATE schema_migrations SET checksum=$2 WHERE name=$1`, entry.Name(), checksum); err != nil {
					return fmt.Errorf("record checksum for migration %s: %w", entry.Name(), err)
				}
			}
			continue
		}
		if err != sql.ErrNoRows {
			return err
		}
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration %s: %w", entry.Name(), err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (name, checksum) VALUES ($1, $2)`, entry.Name(), checksum); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", entry.Name(), err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// ---------- профиль ----------

func (s *Repository) Profile(ctx context.Context) (domain.Profile, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.Profile{}, err
	}
	var p domain.Profile
	err = s.db.QueryRowContext(ctx, `
		SELECT name, surname, occupation, sex, dob, expiry, photo, signature
		FROM user_profiles WHERE user_id=$1`, userID).Scan(
		&p.Name, &p.Surname, &p.Occupation, &p.Sex, &p.DOB, &p.Expiry, &p.Photo, &p.Signature,
	)
	if err == sql.ErrNoRows {
		return domain.Profile{}, fmt.Errorf("profile: %w", domain.ErrNotFound)
	}
	return p, err
}

func (s *Repository) UpdateProfile(ctx context.Context, p domain.Profile) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `
		UPDATE user_profiles
		SET name=$2, surname=$3, occupation=$4, sex=$5, dob=$6, expiry=$7
		WHERE user_id=$1`,
		userID, p.Name, p.Surname, p.Occupation, p.Sex, p.DOB, p.Expiry,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("profile: %w", domain.ErrNotFound)
	}
	return nil
}

func (s *Repository) SetPhoto(ctx context.Context, dataURL string) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE user_profiles SET photo=$2 WHERE user_id=$1`, userID, dataURL)
	return err
}

func (s *Repository) SetSignature(ctx context.Context, dataURL string) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `UPDATE user_profiles SET signature=$2 WHERE user_id=$1`, userID, dataURL)
	return err
}

// ---------- показатели ----------

func scanTrackerWeight(row interface{ Scan(...any) error }) (domain.TrackerWeightEntry, error) {
	var entry domain.TrackerWeightEntry
	err := row.Scan(&entry.Date, &entry.WeightKg, &entry.UpdatedAt)
	return entry, err
}

func scanTrackerWater(row interface{ Scan(...any) error }) (domain.TrackerWaterEntry, error) {
	var entry domain.TrackerWaterEntry
	err := row.Scan(&entry.Date, &entry.Glasses, &entry.GoalGlasses, &entry.UpdatedAt)
	return entry, err
}

func (s *Repository) Trackers(ctx context.Context) (domain.TrackerState, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.TrackerState{}, err
	}
	out := domain.TrackerState{
		WeightHistory: []domain.TrackerWeightEntry{},
		WaterHistory:  []domain.TrackerWaterEntry{},
		CustomHistory: []domain.CustomTrackerEntry{},
	}
	if err := s.db.QueryRowContext(ctx,
		`SELECT water_goal, calorie_goal FROM user_tracker_settings WHERE user_id=$1`, userID,
	).Scan(&out.WaterGoal, &out.CalorieGoal); err != nil {
		return domain.TrackerState{}, err
	}

	weightRows, err := s.db.QueryContext(ctx, `
		SELECT to_char(tracked_on, 'YYYY-MM-DD'), weight_kg::double precision,
		       to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')
		FROM user_tracker_weight_entries
		WHERE user_id=$1
		ORDER BY tracked_on, updated_at`, userID)
	if err != nil {
		return domain.TrackerState{}, err
	}
	for weightRows.Next() {
		entry, err := scanTrackerWeight(weightRows)
		if err != nil {
			weightRows.Close()
			return domain.TrackerState{}, err
		}
		out.WeightHistory = append(out.WeightHistory, entry)
	}
	if err := weightRows.Err(); err != nil {
		weightRows.Close()
		return domain.TrackerState{}, err
	}
	if err := weightRows.Close(); err != nil {
		return domain.TrackerState{}, err
	}
	if len(out.WeightHistory) > 0 {
		current := out.WeightHistory[len(out.WeightHistory)-1].WeightKg
		out.CurrentWeightKg = &current
	}

	waterRows, err := s.db.QueryContext(ctx, `
		SELECT to_char(tracked_on, 'YYYY-MM-DD'), glasses, goal_glasses,
		       to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')
		FROM user_tracker_water_entries
		WHERE user_id=$1
		ORDER BY tracked_on, updated_at`, userID)
	if err != nil {
		return domain.TrackerState{}, err
	}
	defer waterRows.Close()
	for waterRows.Next() {
		entry, err := scanTrackerWater(waterRows)
		if err != nil {
			return domain.TrackerState{}, err
		}
		out.WaterHistory = append(out.WaterHistory, entry)
	}
	if err := waterRows.Err(); err != nil {
		return domain.TrackerState{}, err
	}
	out.CustomTrackers, err = s.CustomTrackers(ctx)
	if err != nil {
		return domain.TrackerState{}, err
	}
	customRows, err := s.db.QueryContext(ctx, `
		SELECT tracker_id, to_char(tracked_on, 'YYYY-MM-DD'),
		       value::double precision, target_value::double precision,
		       to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')
		FROM user_custom_tracker_entries
		WHERE user_id=$1
		ORDER BY tracked_on, tracker_id`, userID)
	if err != nil {
		return domain.TrackerState{}, err
	}
	defer customRows.Close()
	for customRows.Next() {
		var entry domain.CustomTrackerEntry
		if err := customRows.Scan(&entry.TrackerID, &entry.Date, &entry.Value, &entry.TargetValue, &entry.UpdatedAt); err != nil {
			return domain.TrackerState{}, err
		}
		out.CustomHistory = append(out.CustomHistory, entry)
	}
	return out, customRows.Err()
}

func (s *Repository) UpdateCalorieGoal(ctx context.Context, calorieGoal int) (int, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return 0, err
	}
	var saved int
	err = s.db.QueryRowContext(ctx, `
		INSERT INTO user_tracker_settings (user_id, calorie_goal)
		VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET calorie_goal=EXCLUDED.calorie_goal
		RETURNING calorie_goal`, userID, calorieGoal).Scan(&saved)
	return saved, err
}

func (s *Repository) UpsertTrackerWeight(ctx context.Context, date string, weightKg float64) (domain.TrackerWeightEntry, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.TrackerWeightEntry{}, err
	}
	return scanTrackerWeight(s.db.QueryRowContext(ctx, `
		INSERT INTO user_tracker_weight_entries (user_id, tracked_on, weight_kg)
		VALUES ($1, $2::date, $3)
		ON CONFLICT (user_id, tracked_on) DO UPDATE
		SET weight_kg=EXCLUDED.weight_kg, updated_at=now()
		RETURNING to_char(tracked_on, 'YYYY-MM-DD'), weight_kg::double precision,
		          to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')`,
		userID, date, weightKg,
	))
}

func (s *Repository) UpsertTrackerWater(ctx context.Context, date string, glasses, goalGlasses int) (domain.TrackerWaterEntry, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.TrackerWaterEntry{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.TrackerWaterEntry{}, err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO user_tracker_settings (user_id, water_goal) VALUES ($1, $2)
		ON CONFLICT (user_id) DO UPDATE SET water_goal=EXCLUDED.water_goal`, userID, goalGlasses); err != nil {
		return domain.TrackerWaterEntry{}, err
	}
	entry, err := scanTrackerWater(tx.QueryRowContext(ctx, `
		INSERT INTO user_tracker_water_entries (user_id, tracked_on, glasses, goal_glasses)
		VALUES ($1, $2::date, $3, $4)
		ON CONFLICT (user_id, tracked_on) DO UPDATE
		SET glasses=EXCLUDED.glasses,
		    goal_glasses=EXCLUDED.goal_glasses,
		    updated_at=now()
		RETURNING to_char(tracked_on, 'YYYY-MM-DD'), glasses, goal_glasses,
		          to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')`,
		userID, date, glasses, goalGlasses,
	))
	if err != nil {
		return domain.TrackerWaterEntry{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.TrackerWaterEntry{}, err
	}
	return entry, nil
}

// ---------- задачи ----------

const taskSelect = `
	SELECT task.id, task.title, task.description, task.category, task.status,
	       COALESCE(to_char(task.due_date, 'YYYY-MM-DD'), ''),
	       COALESCE(to_char(task.due_time, 'HH24:MI'), ''),
	       COALESCE(to_char(task.reminder_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
	       COALESCE(to_char(task.reminder_sent_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
	       task.priority,
	       to_char(task.created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
	       COALESCE(to_char(task.completed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
	       task.is_milestone,
	       COALESCE(ticktick.sync_status, ''),
	       COALESCE(ticktick.last_error, '')
	FROM tasks AS task
	LEFT JOIN user_ticktick_task_links AS ticktick
	  ON ticktick.task_id=task.id AND ticktick.user_id=task.user_id`

func scanTask(row interface{ Scan(...any) error }) (domain.Task, error) {
	var t domain.Task
	err := row.Scan(&t.ID, &t.Title, &t.Description, &t.Category, &t.Status,
		&t.DueDate, &t.DueTime, &t.ReminderAt, &t.ReminderSentAt, &t.Priority,
		&t.CreatedAt, &t.CompletedAt, &t.IsMilestone,
		&t.TickTickSyncStatus, &t.TickTickSyncError)
	return t, err
}

func (s *Repository) Tasks(ctx context.Context) ([]domain.Task, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, taskSelect+`
		WHERE task.user_id=$1
		ORDER BY CASE task.status WHEN 'todo' THEN 0 ELSE 1 END,
		         COALESCE(task.completed_at, task.created_at) DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Repository) ActiveTasks(ctx context.Context) ([]domain.Task, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, taskSelect+`
		WHERE task.user_id=$1 AND task.status = 'todo'
		ORDER BY task.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Task{}
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Repository) CreateTask(ctx context.Context, in domain.TaskInput) (domain.Task, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	row := s.db.QueryRowContext(ctx, `
		INSERT INTO tasks (
			user_id, title, description, category, status, due_date, due_time,
			reminder_at, reminder_sent_at, completed_at, priority, is_milestone
		)
		VALUES (
			$1, $2, $3, $4, 'todo', NULLIF($5, '')::date, NULLIF($6, '')::time,
			NULLIF($7, '')::timestamptz, NULL, NULL, $8, $9
		)
		RETURNING id, title, description, category, status,
		          COALESCE(to_char(due_date, 'YYYY-MM-DD'), ''),
		          COALESCE(to_char(due_time, 'HH24:MI'), ''),
		          COALESCE(to_char(reminder_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
		          '', priority,
		          to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		          '', is_milestone, '', ''`,
		userID, in.Title, in.Description, in.Category, in.DueDate, in.DueTime,
		in.ReminderAt, in.Priority, in.IsMilestone,
	)
	return scanTask(row)
}

func (s *Repository) UpdateTask(ctx context.Context, id int64, in domain.TaskInput) (domain.Task, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE tasks
		SET title=$3, description=$4, category=$5, status=$6,
		    completed_at=CASE WHEN $6='done' THEN COALESCE(completed_at, now()) ELSE NULL END,
		    due_date=NULLIF($7, '')::date,
		    due_time=NULLIF($8, '')::time,
		    reminder_at=NULLIF($9, '')::timestamptz,
		    reminder_sent_at=CASE WHEN reminder_at IS DISTINCT FROM NULLIF($9, '')::timestamptz THEN NULL ELSE reminder_sent_at END,
		    reminder_claimed_at=NULL,
		    priority=$10,
		    is_milestone=$11
		WHERE id=$1 AND user_id=$2
		RETURNING id, title, description, category, status,
		          COALESCE(to_char(due_date, 'YYYY-MM-DD'), ''),
		          COALESCE(to_char(due_time, 'HH24:MI'), ''),
		          COALESCE(to_char(reminder_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
		          COALESCE(to_char(reminder_sent_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
		          priority,
		          to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		          COALESCE(to_char(completed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
		          is_milestone, '', ''`,
		id, userID, in.Title, in.Description, in.Category, in.Status, in.DueDate,
		in.DueTime, in.ReminderAt, in.Priority, in.IsMilestone,
	)
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return domain.Task{}, fmt.Errorf("task: %w", domain.ErrNotFound)
	}
	return t, err
}

func (s *Repository) SetTaskCompleted(ctx context.Context, id int64, completed bool) (domain.Task, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.Task{}, err
	}
	status := "todo"
	if completed {
		status = "done"
	}
	row := s.db.QueryRowContext(ctx, `
		UPDATE tasks
		SET status=$3,
		    completed_at=CASE WHEN $3='done' THEN COALESCE(completed_at, now()) ELSE NULL END,
		    reminder_claimed_at=NULL
		WHERE id=$1 AND user_id=$2
		RETURNING id, title, description, category, status,
		          COALESCE(to_char(due_date, 'YYYY-MM-DD'), ''),
		          COALESCE(to_char(due_time, 'HH24:MI'), ''),
		          COALESCE(to_char(reminder_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
		          COALESCE(to_char(reminder_sent_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
		          priority,
		          to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		          COALESCE(to_char(completed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), ''),
		          is_milestone, '', ''`, id, userID, status)
	t, err := scanTask(row)
	if err == sql.ErrNoRows {
		return domain.Task{}, fmt.Errorf("task: %w", domain.ErrNotFound)
	}
	return t, err
}

func (s *Repository) DeleteTask(ctx context.Context, id int64) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// JSONB-массив хранит ID как строки. Удаляем ссылку только из проектов владельца.
	if _, err := tx.ExecContext(ctx, `
		UPDATE goals
		SET related_task_ids = related_task_ids - $1::text, updated_at=now()
		WHERE user_id=$2 AND related_task_ids ? $1::text`, strconv.FormatInt(id, 10), userID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("task: %w", domain.ErrNotFound)
	}
	return tx.Commit()
}

// ---------- проекты / достижения ----------

const goalSelect = `
	SELECT id, title, description, summary,
	       current_value, target_value, unit,
	       COALESCE(to_char(deadline, 'YYYY-MM-DD'), ''),
	       related_task_ids, completed_at,
	       COALESCE(to_char(completed_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), ''),
	       pinned, sort_order,
	       to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
	       to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')
	FROM goals`

func scanGoal(row interface{ Scan(...any) error }) (domain.Goal, error) {
	var g domain.Goal
	var relatedJSON []byte
	var completedAt sql.NullTime
	err := row.Scan(
		&g.ID, &g.Title, &g.Description, &g.Summary,
		&g.CurrentValue, &g.TargetValue, &g.Unit, &g.Deadline,
		&relatedJSON, &completedAt, &g.CompletedAt, &g.Pinned, &g.SortOrder,
		&g.CreatedAt, &g.UpdatedAt,
	)
	if err != nil {
		return domain.Goal{}, err
	}
	if err := json.Unmarshal(relatedJSON, &g.RelatedTaskIDs); err != nil {
		return domain.Goal{}, fmt.Errorf("decode related tasks: %w", err)
	}
	if g.RelatedTaskIDs == nil {
		g.RelatedTaskIDs = []string{}
	}
	g.Completed = completedAt.Valid
	if g.TargetValue > 0 {
		g.CompletionPct = minInt(100, int(g.CurrentValue/g.TargetValue*100+0.5))
	}
	return g, nil
}

func (s *Repository) Goal(ctx context.Context, id int64) (domain.Goal, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.Goal{}, err
	}
	g, err := scanGoal(s.db.QueryRowContext(ctx, goalSelect+` WHERE id=$1 AND user_id=$2`, id, userID))
	if err == sql.ErrNoRows {
		return domain.Goal{}, fmt.Errorf("project: %w", domain.ErrNotFound)
	}
	return g, err
}

func (s *Repository) Goals(ctx context.Context) ([]domain.Goal, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, goalSelect+`
		WHERE user_id=$1
		ORDER BY sort_order ASC, id ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Goal{}
	for rows.Next() {
		g, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Repository) Portfolio(ctx context.Context) (domain.Portfolio, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.Portfolio{}, err
	}
	rows, err := s.db.QueryContext(ctx, goalSelect+`
		WHERE user_id=$1 AND completed_at IS NOT NULL
		ORDER BY completed_at DESC, id DESC`, userID)
	if err != nil {
		return domain.Portfolio{}, err
	}
	defer rows.Close()
	p := domain.Portfolio{Pinned: []domain.Goal{}, Completed: []domain.Goal{}}
	for rows.Next() {
		g, err := scanGoal(rows)
		if err != nil {
			return domain.Portfolio{}, err
		}
		p.Completed = append(p.Completed, g)
		if g.Pinned {
			p.Pinned = append(p.Pinned, g)
		}
	}
	if err := rows.Err(); err != nil {
		return domain.Portfolio{}, err
	}
	sort.SliceStable(p.Pinned, func(i, j int) bool {
		if p.Pinned[i].SortOrder == p.Pinned[j].SortOrder {
			return p.Pinned[i].ID < p.Pinned[j].ID
		}
		return p.Pinned[i].SortOrder < p.Pinned[j].SortOrder
	})
	if len(p.Pinned) > 5 {
		p.Pinned = p.Pinned[:5]
	}
	return p, nil
}

func validateRelatedTasks(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, userID int64, ids []string) error {
	for _, raw := range ids {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id <= 0 {
			return fmt.Errorf("related task %q is invalid", raw)
		}
		var exists bool
		if err := q.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM tasks WHERE id=$1 AND user_id=$2)`, id, userID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("related task %d: %w", id, domain.ErrNotFound)
		}
	}
	return nil
}

func ensurePinCapacity(ctx context.Context, tx *sql.Tx, userID, excludeID int64, completed, pinned bool) error {
	if !pinned {
		return nil
	}
	if !completed {
		return fmt.Errorf("only completed projects can be pinned: %w", domain.ErrConflict)
	}
	// Сериализуем операции закрепления, чтобы параллельные запросы не создали шестую запись.
	if _, err := tx.ExecContext(ctx, `LOCK TABLE goals IN SHARE ROW EXCLUSIVE MODE`); err != nil {
		return err
	}
	var count int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM goals WHERE user_id=$1 AND pinned=true AND completed_at IS NOT NULL AND id<>$2`,
		userID, excludeID,
	).Scan(&count); err != nil {
		return err
	}
	if count >= 5 {
		return fmt.Errorf("portfolio already has 5 pinned projects; unpin one first: %w", domain.ErrConflict)
	}
	return nil
}

func (s *Repository) CreateGoal(ctx context.Context, in domain.GoalInput) (domain.Goal, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.Goal{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Goal{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, userID); err != nil {
		return domain.Goal{}, err
	}
	if err := validateRelatedTasks(ctx, tx, userID, in.RelatedTaskIDs); err != nil {
		return domain.Goal{}, err
	}
	if err := ensurePinCapacity(ctx, tx, userID, 0, in.Completed, in.Pinned); err != nil {
		return domain.Goal{}, err
	}
	related, err := json.Marshal(in.RelatedTaskIDs)
	if err != nil {
		return domain.Goal{}, err
	}
	row := tx.QueryRowContext(ctx, `
		INSERT INTO goals (
			user_id, title, description, summary, current_value, target_value, unit,
			deadline, related_task_ids, completed_at, pinned, sort_order
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::date,$9::jsonb,
		        CASE WHEN $10 THEN now() ELSE NULL END,
		        $11 AND $10,
		        COALESCE((SELECT MAX(sort_order) + 1 FROM goals WHERE user_id=$1), 1))
		RETURNING id, title, description, summary,
		          current_value, target_value, unit,
		          COALESCE(to_char(deadline, 'YYYY-MM-DD'), ''),
		          related_task_ids, completed_at,
		          COALESCE(to_char(completed_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), ''),
		          pinned, sort_order,
		          to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		          to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')`,
		userID, in.Title, in.Description, in.Summary, in.CurrentValue, in.TargetValue,
		in.Unit, in.Deadline, related, in.Completed, in.Pinned,
	)
	g, err := scanGoal(row)
	if err != nil {
		return domain.Goal{}, err
	}
	return g, tx.Commit()
}

func (s *Repository) UpdateGoal(ctx context.Context, id int64, in domain.GoalInput) (domain.Goal, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.Goal{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.Goal{}, err
	}
	defer tx.Rollback()
	if err := validateRelatedTasks(ctx, tx, userID, in.RelatedTaskIDs); err != nil {
		return domain.Goal{}, err
	}
	if err := ensurePinCapacity(ctx, tx, userID, id, in.Completed, in.Pinned); err != nil {
		return domain.Goal{}, err
	}
	related, err := json.Marshal(in.RelatedTaskIDs)
	if err != nil {
		return domain.Goal{}, err
	}
	row := tx.QueryRowContext(ctx, `
		UPDATE goals
		SET title=$3, description=$4, summary=$5,
		    current_value=$6, target_value=$7, unit=$8,
		    deadline=NULLIF($9,'')::date, related_task_ids=$10::jsonb,
		    completed_at=CASE WHEN $11 THEN COALESCE(completed_at, now()) ELSE NULL END,
		    pinned=$12 AND $11,
		    updated_at=now()
		WHERE id=$1 AND user_id=$2
		RETURNING id, title, description, summary,
		          current_value, target_value, unit,
		          COALESCE(to_char(deadline, 'YYYY-MM-DD'), ''),
		          related_task_ids, completed_at,
		          COALESCE(to_char(completed_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'), ''),
		          pinned, sort_order,
		          to_char(created_at, 'YYYY-MM-DD"T"HH24:MI:SSOF'),
		          to_char(updated_at, 'YYYY-MM-DD"T"HH24:MI:SSOF')`,
		id, userID, in.Title, in.Description, in.Summary,
		in.CurrentValue, in.TargetValue, in.Unit, in.Deadline, related,
		in.Completed, in.Pinned,
	)
	g, err := scanGoal(row)
	if err == sql.ErrNoRows {
		return domain.Goal{}, fmt.Errorf("project: %w", domain.ErrNotFound)
	}
	if err != nil {
		return domain.Goal{}, err
	}
	return g, tx.Commit()
}

func (s *Repository) DeleteGoal(ctx context.Context, id int64) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM goals WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("project: %w", domain.ErrNotFound)
	}
	return nil
}

func (s *Repository) ReorderGoals(ctx context.Context, ids []int64) error {
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
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM goals WHERE user_id=$1`, userID).Scan(&count); err != nil {
		return err
	}
	if count != len(ids) {
		return fmt.Errorf("project order is stale; refresh projects and try again: %w", domain.ErrConflict)
	}
	for index, id := range ids {
		result, err := tx.ExecContext(ctx, `UPDATE goals SET sort_order=$3 WHERE id=$1 AND user_id=$2`, id, userID, index+1)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return fmt.Errorf("project order contains an unavailable project: %w", domain.ErrConflict)
		}
	}
	return tx.Commit()
}

// Reset удаляет только задачи и проекты текущего пользователя. Профиль,
// фотография и личная история показателей сохраняются.
func (s *Repository) Reset(ctx context.Context) error {
	userID, err := currentUserID(ctx)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM goals WHERE user_id=$1`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tasks WHERE user_id=$1`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// compile-time check: *sql.Tx satisfies the small query interface used above.
var _ interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
} = (*sql.Tx)(nil)
