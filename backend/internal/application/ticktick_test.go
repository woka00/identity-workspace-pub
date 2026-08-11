package application

import (
	"context"
	"errors"
	"testing"

	"identity-workspace/internal/domain"
)

type tickTickRepoStub struct {
	Repository
	connection     domain.TickTickConnection
	link           domain.TickTickTaskLink
	hasLink        bool
	task           domain.Task
	remoteSnapshot []domain.TickTickRemoteTask
	pullResult     domain.TickTickPullResult
}

func (r *tickTickRepoStub) CreateTask(_ context.Context, in domain.TaskInput) (domain.Task, error) {
	r.task = domain.Task{
		ID: 1, Title: in.Title, Description: in.Description, Category: in.Category,
		Status: "todo", DueDate: in.DueDate, IsMilestone: in.IsMilestone,
	}
	return r.task, nil
}

func (r *tickTickRepoStub) SetTaskCompleted(_ context.Context, _ int64, completed bool) (domain.Task, error) {
	if completed {
		r.task.Status = "done"
	} else {
		r.task.Status = "todo"
	}
	return r.task, nil
}

func (r *tickTickRepoStub) TickTickConnection(context.Context) (domain.TickTickConnection, error) {
	return r.connection, nil
}

func (r *tickTickRepoStub) TickTickTaskLink(_ context.Context, taskID int64) (domain.TickTickTaskLink, error) {
	if !r.hasLink || r.link.TaskID != taskID {
		return domain.TickTickTaskLink{}, domain.ErrNotFound
	}
	return r.link, nil
}

func (r *tickTickRepoStub) SaveTickTickTaskLink(_ context.Context, link domain.TickTickTaskLink) error {
	r.link = link
	r.hasLink = true
	return nil
}

func (r *tickTickRepoStub) TickTickPendingTasks(context.Context) ([]domain.Task, error) {
	return nil, nil
}

func (r *tickTickRepoStub) ApplyTickTickSnapshot(_ context.Context, tasks []domain.TickTickRemoteTask) (domain.TickTickPullResult, error) {
	r.remoteSnapshot = append([]domain.TickTickRemoteTask(nil), tasks...)
	return r.pullResult, nil
}

type tickTickGatewayStub struct {
	createTask   domain.Task
	updateTask   domain.Task
	created      int
	updated      int
	completed    int
	createErr    error
	remoteTaskID string
	remoteTasks  []domain.TickTickRemoteTask
	listErr      error
}

func (g *tickTickGatewayStub) Configured() bool                   { return true }
func (g *tickTickGatewayStub) AuthorizeURL(string, string) string { return "" }
func (g *tickTickGatewayStub) AccessToken(context.Context, string, string) (string, error) {
	return "", nil
}
func (g *tickTickGatewayStub) EnsureProject(context.Context, string) (string, string, error) {
	return "", "", nil
}
func (g *tickTickGatewayStub) ListTasks(context.Context, string) ([]domain.TickTickRemoteTask, error) {
	return g.remoteTasks, g.listErr
}
func (g *tickTickGatewayStub) CreateTask(_ context.Context, _, _ string, task domain.Task) (string, error) {
	g.created++
	g.createTask = task
	if g.createErr != nil {
		return "", g.createErr
	}
	if g.remoteTaskID == "" {
		g.remoteTaskID = "ticktick-task-1"
	}
	return g.remoteTaskID, nil
}
func (g *tickTickGatewayStub) UpdateTask(_ context.Context, _, _, _ string, task domain.Task) error {
	g.updated++
	g.updateTask = task
	return nil
}
func (g *tickTickGatewayStub) CompleteTask(context.Context, string, string, string) error {
	g.completed++
	return nil
}
func (g *tickTickGatewayStub) DeleteTask(context.Context, string, string, string) error { return nil }

func TestTaskCreationAndCompletionSyncToTickTick(t *testing.T) {
	repo := &tickTickRepoStub{connection: domain.TickTickConnection{
		AccessToken: "access", ProjectID: "project", ProjectName: "AVATAR.ID",
	}}
	gateway := &tickTickGatewayStub{}
	service := New(repo, nil, nil).WithTickTick(gateway)

	created, err := service.CreateTask(context.Background(), domain.TaskInput{
		Title: "Название", Description: "Описание", Category: "Работа", DueDate: "2026-08-10",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gateway.created != 1 || gateway.createTask.Title != "Название" || gateway.createTask.Description != "Описание" {
		t.Fatalf("task was not created in TickTick correctly: %#v", gateway.createTask)
	}
	if created.TickTickSyncStatus != "synced" || repo.link.TickTickTaskID != "ticktick-task-1" {
		t.Fatalf("unexpected local sync state: task=%#v link=%#v", created, repo.link)
	}

	completed, err := service.SetTaskCompleted(context.Background(), created.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if gateway.updated != 1 || gateway.completed != 1 {
		t.Fatalf("expected one update and one complete command, got update=%d complete=%d", gateway.updated, gateway.completed)
	}
	if completed.Status != "done" || completed.TickTickSyncStatus != "synced" {
		t.Fatalf("unexpected completed task: %#v", completed)
	}
}

func TestTickTickFailureDoesNotFailLocalTaskCreation(t *testing.T) {
	repo := &tickTickRepoStub{connection: domain.TickTickConnection{AccessToken: "access", ProjectID: "project"}}
	gateway := &tickTickGatewayStub{createErr: errors.New("temporary TickTick outage")}
	service := New(repo, nil, nil).WithTickTick(gateway)

	created, err := service.CreateTask(context.Background(), domain.TaskInput{Title: "Локальная задача", Category: "Личное"})
	if err != nil {
		t.Fatalf("local task creation must succeed: %v", err)
	}
	if created.ID != 1 || created.TickTickSyncStatus != "error" || created.TickTickSyncError == "" {
		t.Fatalf("unexpected task after remote failure: %#v", created)
	}
	if !repo.hasLink || repo.link.SyncStatus != "error" {
		t.Fatalf("failed sync must remain retryable: %#v", repo.link)
	}
}

func TestSyncTickTickImportsRemoteSnapshot(t *testing.T) {
	repo := &tickTickRepoStub{
		connection: domain.TickTickConnection{AccessToken: "access", ProjectID: "avatar-project"},
		pullResult: domain.TickTickPullResult{Imported: 2, Updated: 1, Completed: 1},
	}
	gateway := &tickTickGatewayStub{remoteTasks: []domain.TickTickRemoteTask{
		{ID: "remote-1", ProjectID: "work", Title: "  Первая  ", Description: "Описание", DueDate: "2026-08-10"},
		{ID: "remote-2", ProjectID: "home", Title: "Вторая", Description: ""},
	}}
	service := New(repo, nil, nil).WithTickTick(gateway)

	result, err := service.SyncTickTick(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 || result.Updated != 1 || result.Completed != 1 {
		t.Fatalf("unexpected sync result: %#v", result)
	}
	if len(repo.remoteSnapshot) != 2 || repo.remoteSnapshot[0].Title != "Первая" || repo.remoteSnapshot[0].Status != "todo" {
		t.Fatalf("unexpected normalized snapshot: %#v", repo.remoteSnapshot)
	}
}
