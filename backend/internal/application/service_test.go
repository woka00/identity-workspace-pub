package application

import (
	"context"
	"testing"

	"avatar-id/internal/domain"
)

type stateRepositoryStub struct {
	Repository
	activeTaskCalls int
}

func (r *stateRepositoryStub) Profile(context.Context) (domain.Profile, error) {
	return domain.Profile{}, nil
}

func (r *stateRepositoryStub) BottomNavigation(context.Context) ([]string, error) {
	return []string{"card", "tasks", "tracker", "profile"}, nil
}

func (r *stateRepositoryStub) WorkspacePreferences(context.Context) (domain.WorkspacePreferences, error) {
	return domain.WorkspacePreferences{}, nil
}

func (r *stateRepositoryStub) ActiveTasks(context.Context) ([]domain.Task, error) {
	r.activeTaskCalls++
	return []domain.Task{}, nil
}

func TestStateCanSkipDuplicateActiveTaskQuery(t *testing.T) {
	repository := &stateRepositoryStub{}
	service := New(repository, nil, nil)
	state, err := service.StateWithoutActiveTasks(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if repository.activeTaskCalls != 0 || state.ActiveTasks == nil {
		t.Fatalf("active task calls=%d tasks=%v", repository.activeTaskCalls, state.ActiveTasks)
	}
	if _, err := service.State(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repository.activeTaskCalls != 1 {
		t.Fatalf("default state active task calls=%d, want 1", repository.activeTaskCalls)
	}
}
