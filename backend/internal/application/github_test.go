package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"avatar-id/internal/domain"
)

type githubRepoStub struct {
	Repository
	record domain.GitHubTrackerRecord
}

func (r *githubRepoStub) GitHubTracker(context.Context) (domain.GitHubTrackerRecord, error) {
	if r.record.Username == "" {
		return domain.GitHubTrackerRecord{}, domain.ErrNotFound
	}
	return r.record, nil
}

func (r *githubRepoStub) UpsertGitHubTracker(_ context.Context, username string) error {
	if r.record.Username != username {
		r.record = domain.GitHubTrackerRecord{Username: username}
	}
	return nil
}

func (r *githubRepoStub) CacheGitHubTracker(_ context.Context, username string, state domain.GitHubTrackerState) error {
	r.record = domain.GitHubTrackerRecord{Username: username, Activity: state, FetchedAt: state.FetchedAt}
	return nil
}

func (r *githubRepoStub) DeleteGitHubTracker(context.Context) error {
	r.record = domain.GitHubTrackerRecord{}
	return nil
}

type githubGatewayStub struct {
	days  []domain.GitHubActivityDay
	err   error
	calls int
}

func (g *githubGatewayStub) PublicContributions(context.Context, string) ([]domain.GitHubActivityDay, error) {
	g.calls++
	return g.days, g.err
}

func TestNormalizeGitHubUsername(t *testing.T) {
	for _, valid := range []string{"octocat", "github-user", "A1"} {
		if _, err := NormalizeGitHubUsername(valid); err != nil {
			t.Fatalf("valid username %q rejected: %v", valid, err)
		}
	}
	for _, invalid := range []string{"", "-name", "name-", "name--two", "имя", "name space"} {
		if _, err := NormalizeGitHubUsername(invalid); err == nil {
			t.Fatalf("invalid username %q accepted", invalid)
		}
	}
}

func TestGitHubTrackerUsesFormalDayAndCountsCurrentWeek(t *testing.T) {
	location := time.FixedZone("MSK", 3*60*60)
	now := time.Date(2026, 8, 12, 1, 30, 0, 0, location)
	days := []domain.GitHubActivityDay{
		{Date: "2025-08-11", Count: 100, Level: 4},
		{Date: "2025-08-12", Count: 4, Level: 3},
		{Date: "2026-08-10", Count: 3, Level: 2},
		{Date: "2026-08-11", Count: 2, Level: 1},
		{Date: "2026-08-12", Count: 8, Level: 4},
	}
	state := buildGitHubTracker("octocat", days, now)
	if state.WeekContributions != 5 {
		t.Fatalf("week contributions=%d, want 5", state.WeekContributions)
	}
	if state.TodayContributions != 2 {
		t.Fatalf("today contributions=%d, want 2", state.TodayContributions)
	}
	if state.YearContributions != 9 {
		t.Fatalf("year contributions=%d, want 9", state.YearContributions)
	}
	if len(state.Days) != 371 {
		t.Fatalf("days=%d, want 371", len(state.Days))
	}
	var tuesday domain.GitHubActivityDay
	for _, day := range state.Days {
		if day.Date == "2026-08-11" {
			tuesday = day
		}
	}
	if tuesday.Count != 2 || tuesday.Level != 1 {
		t.Fatalf("formal day mismatch: %#v", tuesday)
	}
}

func TestGitHubTrackerReturnsStaleCacheWhenProviderFails(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	cached := buildGitHubTracker("octocat", nil, now.Add(-time.Hour))
	repo := &githubRepoStub{record: domain.GitHubTrackerRecord{Username: "octocat", Activity: cached, FetchedAt: cached.FetchedAt}}
	gateway := &githubGatewayStub{err: errors.New("rate limited")}
	service := New(repo, nil, func() time.Time { return now }).WithGitHub(gateway)
	state, err := service.GitHubTracker(context.Background(), false)
	if err != nil || !state.Stale || !state.Configured {
		t.Fatalf("unexpected stale result: state=%#v err=%v", state, err)
	}
}

func TestSaveGitHubTrackerValidatesRemoteUserBeforePersisting(t *testing.T) {
	location := time.FixedZone("MSK", 3*60*60)
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, location)
	repo := &githubRepoStub{}
	gateway := &githubGatewayStub{err: domain.ErrNotFound}
	service := New(repo, nil, func() time.Time { return now }).WithGitHub(gateway)

	if _, err := service.SaveGitHubTracker(context.Background(), "missing-user"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("save error=%v, want not found", err)
	}
	if repo.record.Username != "" {
		t.Fatalf("invalid username was persisted: %q", repo.record.Username)
	}
	if gateway.calls != 1 {
		t.Fatalf("gateway calls=%d, want 1", gateway.calls)
	}
}

func TestGitHubTrackerInvalidatesLegacyActivityCache(t *testing.T) {
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	cached := buildGitHubTracker("octocat", nil, now.Add(-time.Minute))
	cached.Source = ""
	repo := &githubRepoStub{record: domain.GitHubTrackerRecord{Username: "octocat", Activity: cached, FetchedAt: cached.FetchedAt}}
	gateway := &githubGatewayStub{days: []domain.GitHubActivityDay{{Date: "2026-08-12", Count: 8, Level: 3}}}
	service := New(repo, nil, func() time.Time { return now }).WithGitHub(gateway)

	state, err := service.GitHubTracker(context.Background(), false)
	if err != nil || gateway.calls != 1 || state.WeekContributions != 8 || state.Source != githubActivitySource {
		t.Fatalf("legacy cache was not refreshed: state=%#v calls=%d err=%v", state, gateway.calls, err)
	}
}
