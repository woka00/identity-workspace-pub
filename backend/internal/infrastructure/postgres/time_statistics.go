package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"avatar-id/internal/domain"
)

type storedTimeSession struct {
	id           int64
	activityID   int64
	activityName string
	startedAt    time.Time
	endedAt      time.Time
	active       bool
}

func (s *Repository) loadTimeSessions(ctx context.Context, userID int64, start, end, now time.Time, activityID int64) ([]storedTimeSession, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT session.id, activity.id, activity.name, session.started_at,
		       least(COALESCE(session.ended_at, $4), $3), session.ended_at IS NULL
		FROM user_time_sessions AS session
		JOIN user_time_activities AS activity
		  ON activity.id=session.activity_id AND activity.user_id=session.user_id
		WHERE session.user_id=$1
		  AND session.started_at < $3
		  AND COALESCE(session.ended_at, $4) > $2
		  AND ($5=0 OR activity.id=$5)
		ORDER BY session.started_at DESC, session.id DESC`, userID, start, end, now, activityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []storedTimeSession{}
	for rows.Next() {
		var session storedTimeSession
		if err := rows.Scan(
			&session.id,
			&session.activityID,
			&session.activityName,
			&session.startedAt,
			&session.endedAt,
			&session.active,
		); err != nil {
			return nil, err
		}
		out = append(out, session)
	}
	return out, rows.Err()
}

func laterTime(left, right time.Time) time.Time {
	if left.After(right) {
		return left
	}
	return right
}

func earlierTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func elapsedSeconds(start, end time.Time) int64 {
	if !end.After(start) {
		return 0
	}
	return int64(end.Sub(start) / time.Second)
}

func splitTimeSession(session storedTimeSession, start, end time.Time, location *time.Location) []domain.TimeStatisticsSession {
	segmentStart := laterTime(session.startedAt, start)
	boundedEnd := earlierTime(session.endedAt, end)
	if boundedEnd.Before(segmentStart) {
		return nil
	}
	out := []domain.TimeStatisticsSession{}
	for !segmentStart.After(boundedEnd) {
		localStart := segmentStart.In(location)
		nextDay := time.Date(localStart.Year(), localStart.Month(), localStart.Day()+1, 0, 0, 0, 0, location)
		segmentEnd := earlierTime(boundedEnd, nextDay)
		endedAt := segmentEnd.Format(time.RFC3339Nano)
		active := session.active && !segmentEnd.Before(boundedEnd)
		if active {
			endedAt = ""
		}
		out = append(out, domain.TimeStatisticsSession{
			ID:              session.id,
			ActivityID:      session.activityID,
			ActivityName:    session.activityName,
			Date:            localStart.Format("2006-01-02"),
			StartedAt:       segmentStart.Format(time.RFC3339Nano),
			EndedAt:         endedAt,
			DurationSeconds: elapsedSeconds(segmentStart, segmentEnd),
			Active:          active,
		})
		if !segmentEnd.Before(boundedEnd) {
			break
		}
		segmentStart = segmentEnd
	}
	return out
}

func sortStatisticsSessions(sessions []domain.TimeStatisticsSession) {
	sort.SliceStable(sessions, func(left, right int) bool {
		leftTime, leftErr := time.Parse(time.RFC3339Nano, sessions[left].StartedAt)
		rightTime, rightErr := time.Parse(time.RFC3339Nano, sessions[right].StartedAt)
		if leftErr == nil && rightErr == nil && !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		return sessions[left].ID > sessions[right].ID
	})
}

func (s *Repository) TimeStatistics(ctx context.Context, trackerRange domain.TimeTrackerRange) (domain.TimeStatistics, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.TimeStatistics{}, err
	}
	sessions, err := s.loadTimeSessions(
		ctx,
		userID,
		trackerRange.PeriodStart,
		trackerRange.PeriodEnd,
		trackerRange.Now,
		0,
	)
	if err != nil {
		return domain.TimeStatistics{}, err
	}
	statistics := domain.TimeStatistics{
		Date:        trackerRange.Date,
		Period:      trackerRange.Period,
		PeriodStart: trackerRange.PeriodStart.Format("2006-01-02"),
		PeriodEnd:   trackerRange.PeriodEnd.AddDate(0, 0, -1).Format("2006-01-02"),
		Activities:  []domain.TimeStatisticsActivity{},
		Days:        []domain.TimeStatisticsDay{},
		Sessions:    []domain.TimeStatisticsSession{},
	}
	dayTotals := map[string]int64{}
	for day := trackerRange.PeriodStart; day.Before(trackerRange.PeriodEnd); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		dayTotals[key] = 0
		statistics.Days = append(statistics.Days, domain.TimeStatisticsDay{Date: key})
	}
	activityIndex := map[int64]int{}
	for _, session := range sessions {
		boundedStart := laterTime(session.startedAt, trackerRange.PeriodStart)
		boundedEnd := earlierTime(session.endedAt, trackerRange.PeriodEnd)
		duration := elapsedSeconds(boundedStart, boundedEnd)
		statistics.TotalSeconds += duration
		statistics.SessionCount++
		if duration > statistics.LongestSessionSeconds {
			statistics.LongestSessionSeconds = duration
		}
		index, exists := activityIndex[session.activityID]
		if !exists {
			index = len(statistics.Activities)
			activityIndex[session.activityID] = index
			statistics.Activities = append(statistics.Activities, domain.TimeStatisticsActivity{
				ID:   session.activityID,
				Name: session.activityName,
			})
		}
		statistics.Activities[index].TotalSeconds += duration
		statistics.Activities[index].SessionCount++
		segments := splitTimeSession(session, trackerRange.PeriodStart, trackerRange.PeriodEnd, trackerRange.Now.Location())
		for _, segment := range segments {
			dayTotals[segment.Date] += segment.DurationSeconds
			statistics.Sessions = append(statistics.Sessions, segment)
		}
	}
	statistics.ActivityCount = len(statistics.Activities)
	for index := range statistics.Activities {
		if statistics.TotalSeconds > 0 {
			statistics.Activities[index].SharePercent = int((statistics.Activities[index].TotalSeconds*100 + statistics.TotalSeconds/2) / statistics.TotalSeconds)
		}
	}
	sort.SliceStable(statistics.Activities, func(left, right int) bool {
		if statistics.Activities[left].TotalSeconds == statistics.Activities[right].TotalSeconds {
			return statistics.Activities[left].Name < statistics.Activities[right].Name
		}
		return statistics.Activities[left].TotalSeconds > statistics.Activities[right].TotalSeconds
	})
	for index := range statistics.Days {
		statistics.Days[index].TotalSeconds = dayTotals[statistics.Days[index].Date]
		if statistics.Days[index].TotalSeconds > 0 {
			statistics.ActiveDays++
		}
	}
	sortStatisticsSessions(statistics.Sessions)
	return statistics, nil
}

func (s *Repository) TimeActivityStatistics(ctx context.Context, id int64, trackerRange domain.TimeActivityStatisticsRange) (domain.TimeActivityStatistics, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return domain.TimeActivityStatistics{}, err
	}
	var name string
	err = s.db.QueryRowContext(ctx, `
		SELECT name FROM user_time_activities
		WHERE id=$1 AND user_id=$2`, id, userID).Scan(&name)
	if err == sql.ErrNoRows {
		return domain.TimeActivityStatistics{}, fmt.Errorf("time activity: %w", domain.ErrNotFound)
	}
	if err != nil {
		return domain.TimeActivityStatistics{}, err
	}
	start := time.Date(1970, time.January, 1, 0, 0, 0, 0, trackerRange.Now.Location())
	sessions, err := s.loadTimeSessions(ctx, userID, start, trackerRange.Now, trackerRange.Now, id)
	if err != nil {
		return domain.TimeActivityStatistics{}, err
	}
	statistics := domain.TimeActivityStatistics{
		ActivityID: id,
		Name:       name,
		Sessions:   []domain.TimeStatisticsSession{},
	}
	for _, session := range sessions {
		duration := elapsedSeconds(session.startedAt, session.endedAt)
		statistics.TotalSeconds += duration
		statistics.SessionCount++
		weekStart := laterTime(session.startedAt, trackerRange.WeekStart)
		statistics.WeekSeconds += elapsedSeconds(weekStart, session.endedAt)
		statistics.Sessions = append(
			statistics.Sessions,
			splitTimeSession(session, start, trackerRange.Now, trackerRange.Now.Location())...,
		)
	}
	if statistics.SessionCount > 0 {
		statistics.AverageSessionSeconds = statistics.TotalSeconds / int64(statistics.SessionCount)
	}
	sortStatisticsSessions(statistics.Sessions)
	return statistics, nil
}
