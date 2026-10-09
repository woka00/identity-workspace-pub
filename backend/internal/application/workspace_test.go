package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"avatar-id/internal/domain"
)

type workspaceRepositoryStub struct {
	Repository
	saved *domain.WorkspacePreferences
	err   error
}

func (r *workspaceRepositoryStub) UpdateWorkspacePreferences(_ context.Context, value domain.WorkspacePreferences) error {
	r.saved = &value
	return r.err
}

func TestUpdateWorkspacePreferences(t *testing.T) {
	for _, features := range [][]string{{"tracker"}, {"widgets"}, {"tracker", "calories", "tasks", "projects", "widgets"}} {
		r := &workspaceRepositoryStub{}
		s := &Service{repo: r}
		preferences := domain.WorkspacePreferences{Features: features, OnboardingCompleted: true, TimerIntroSeen: true}
		if err := s.UpdateWorkspacePreferences(context.Background(), preferences); err != nil {
			t.Fatal(err)
		}
		if r.saved == nil || !reflect.DeepEqual(*r.saved, preferences) {
			t.Fatalf("preferences not preserved: %#v", r.saved)
		}
	}
}

func TestUpdateWorkspacePreferencesRejectsInvalidSelection(t *testing.T) {
	for _, preferences := range []domain.WorkspacePreferences{
		{Features: []string{"tracker"}},
		{OnboardingCompleted: true},
		{Features: []string{}, OnboardingCompleted: true},
		{Features: []string{"unknown"}, OnboardingCompleted: true},
		{Features: []string{"tracker", "tracker"}, OnboardingCompleted: true},
		{Features: []string{"tracker", "calories", "tasks", "projects", "widgets", "card"}, OnboardingCompleted: true},
	} {
		r := &workspaceRepositoryStub{}
		s := &Service{repo: r}
		if err := s.UpdateWorkspacePreferences(context.Background(), preferences); err == nil {
			t.Fatalf("accepted invalid preferences: %#v", preferences)
		}
		if r.saved != nil {
			t.Fatal("invalid preferences reached persistence")
		}
	}
}

func TestUpdateWorkspacePreferencesReturnsPersistenceError(t *testing.T) {
	want := errors.New("storage unavailable")
	s := &Service{repo: &workspaceRepositoryStub{err: want}}
	err := s.UpdateWorkspacePreferences(context.Background(), domain.WorkspacePreferences{Features: []string{"tasks"}, OnboardingCompleted: true})
	if !errors.Is(err, want) {
		t.Fatalf("got %v, want %v", err, want)
	}
}
