package application

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"avatar-id/internal/domain"
)

const (
	githubActivityCacheTTL  = 30 * time.Minute
	githubForceRefreshFloor = time.Minute
	githubCalendarWeeks     = 53
	githubActivitySource    = "profile-contributions-v2"
)

var githubUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?$`)

type GitHubGateway interface {
	PublicContributions(context.Context, string) ([]domain.GitHubActivityDay, error)
}

func NormalizeGitHubUsername(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 39 || strings.Contains(value, "--") || !githubUsernamePattern.MatchString(value) {
		return "", invalidf("GitHub username должен содержать от 1 до 39 латинских букв, цифр или одиночных дефисов")
	}
	return value, nil
}

func (s *Service) GitHubTracker(ctx context.Context, force bool) (domain.GitHubTrackerState, error) {
	record, err := s.repo.GitHubTracker(ctx)
	if errors.Is(err, domain.ErrNotFound) {
		return emptyGitHubTracker(s.now()), nil
	}
	if err != nil {
		return domain.GitHubTrackerState{}, err
	}
	return s.githubTrackerFromRecord(ctx, record, force)
}

func (s *Service) SaveGitHubTracker(ctx context.Context, username string) (domain.GitHubTrackerState, error) {
	username, err := NormalizeGitHubUsername(username)
	if err != nil {
		return domain.GitHubTrackerState{}, err
	}
	if s.github == nil {
		return domain.GitHubTrackerState{}, fmt.Errorf("github gateway is unavailable: %w", domain.ErrConflict)
	}
	now := s.now()
	days, err := s.github.PublicContributions(ctx, username)
	if err != nil {
		return domain.GitHubTrackerState{}, err
	}
	activity := buildGitHubTracker(username, days, now)
	if err := s.repo.UpsertGitHubTracker(ctx, username); err != nil {
		return domain.GitHubTrackerState{}, err
	}
	if err := s.repo.CacheGitHubTracker(ctx, username, activity); err != nil {
		return domain.GitHubTrackerState{}, err
	}
	return activity, nil
}

func (s *Service) DeleteGitHubTracker(ctx context.Context) error {
	return s.repo.DeleteGitHubTracker(ctx)
}

func (s *Service) githubTrackerFromRecord(ctx context.Context, record domain.GitHubTrackerRecord, force bool) (domain.GitHubTrackerState, error) {
	now := s.now()
	fetchedAt, _ := time.Parse(time.RFC3339, record.FetchedAt)
	age := now.UTC().Sub(fetchedAt)
	hasCache := record.Activity.Source == githubActivitySource && len(record.Activity.Days) == githubCalendarWeeks*7
	if hasCache && age >= 0 && (!force && age < githubActivityCacheTTL || force && age < githubForceRefreshFloor) {
		cached := record.Activity
		cached.Configured = true
		cached.Username = record.Username
		cached.Stale = false
		return cached, nil
	}
	if s.github == nil {
		return domain.GitHubTrackerState{}, fmt.Errorf("github gateway is unavailable: %w", domain.ErrConflict)
	}
	days, err := s.github.PublicContributions(ctx, record.Username)
	if err != nil {
		if hasCache {
			cached := record.Activity
			cached.Configured = true
			cached.Username = record.Username
			cached.Stale = true
			return cached, nil
		}
		return domain.GitHubTrackerState{}, err
	}
	activity := buildGitHubTracker(record.Username, days, now)
	if err := s.repo.CacheGitHubTracker(ctx, record.Username, activity); err != nil {
		return domain.GitHubTrackerState{}, err
	}
	return activity, nil
}

func emptyGitHubTracker(now time.Time) domain.GitHubTrackerState {
	state := buildGitHubTracker("", nil, now)
	state.Configured = false
	return state
}

func buildGitHubTracker(username string, contributions []domain.GitHubActivityDay, now time.Time) domain.GitHubTrackerState {
	start := githubCalendarStart(now)
	formalToday := formalGitHubDate(now)
	counts := make(map[string]int, githubCalendarWeeks*7)
	levels := make(map[string]int, githubCalendarWeeks*7)
	for _, contribution := range contributions {
		if contribution.Count < 0 {
			continue
		}
		if _, err := time.Parse("2006-01-02", contribution.Date); err != nil {
			continue
		}
		counts[contribution.Date] = contribution.Count
		levels[contribution.Date] = contribution.Level
	}

	weekStart := formalToday.AddDate(0, 0, -((int(formalToday.Weekday()) + 6) % 7))
	yearStart := formalToday.AddDate(0, 0, -364)
	state := domain.GitHubTrackerState{
		Configured: strings.TrimSpace(username) != "",
		Username:   username,
		Days:       make([]domain.GitHubActivityDay, 0, githubCalendarWeeks*7),
		FetchedAt:  now.UTC().Format(time.RFC3339),
		Source:     githubActivitySource,
	}
	for index := 0; index < githubCalendarWeeks*7; index++ {
		date := start.AddDate(0, 0, index)
		key := date.Format("2006-01-02")
		count := counts[key]
		level := levels[key]
		if level < 0 || level > 4 || count > 0 && level == 0 {
			level = githubActivityLevel(count)
		}
		state.Days = append(state.Days, domain.GitHubActivityDay{Date: key, Count: count, Level: level})
		if date.Equal(formalToday) {
			state.TodayContributions = count
		}
		if !date.Before(weekStart) && !date.After(formalToday) {
			state.WeekContributions += count
		}
		if !date.Before(yearStart) && !date.After(formalToday) {
			state.YearContributions += count
		}
	}
	return state
}

func formalGitHubDate(value time.Time) time.Time {
	shifted := value.Add(-3 * time.Hour)
	return time.Date(shifted.Year(), shifted.Month(), shifted.Day(), 0, 0, 0, 0, shifted.Location())
}

func githubCalendarStart(now time.Time) time.Time {
	today := formalGitHubDate(now)
	weekStart := today.AddDate(0, 0, -((int(today.Weekday()) + 6) % 7))
	return weekStart.AddDate(0, 0, -(githubCalendarWeeks-1)*7)
}

func githubActivityLevel(count int) int {
	switch {
	case count <= 0:
		return 0
	case count == 1:
		return 1
	case count <= 3:
		return 2
	case count <= 6:
		return 3
	default:
		return 4
	}
}
