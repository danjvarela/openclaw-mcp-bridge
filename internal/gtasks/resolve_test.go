package gtasks_test

import (
	"context"
	"errors"
	"testing"

	"github.com/danjvarela/openclaw-mcp-bridge/internal/gtasks"
)

type fakeAPI struct {
	ensureCalls int
	ensureTitle string
	ensureID    string
	ensureErr   error
}

func (f *fakeAPI) ListTaskLists(context.Context) ([]gtasks.TaskList, error) { return nil, nil }

func (f *fakeAPI) EnsureTaskList(_ context.Context, title string) (string, error) {
	f.ensureCalls++
	f.ensureTitle = title
	if f.ensureErr != nil {
		return "", f.ensureErr
	}
	return f.ensureID, nil
}

func (f *fakeAPI) InsertTask(context.Context, string, gtasks.TaskInput) (*gtasks.Task, error) {
	return &gtasks.Task{}, nil
}

func (f *fakeAPI) UpdateTask(context.Context, string, string, gtasks.TaskInput) (*gtasks.Task, error) {
	return &gtasks.Task{}, nil
}

func (f *fakeAPI) DeleteTask(context.Context, string, string) error { return nil }

func (f *fakeAPI) ListTasks(context.Context, string, gtasks.ListTasksOptions) ([]gtasks.Task, error) {
	return nil, nil
}

func (f *fakeAPI) CompleteTask(context.Context, string, string) (*gtasks.Task, error) {
	return &gtasks.Task{}, nil
}

func TestResolveTaskList(t *testing.T) {
	f := &fakeAPI{ensureID: "bot-list-id"}
	id, err := gtasks.ResolveTaskList(context.Background(), f)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "bot-list-id" {
		t.Fatalf("id = %q, want %q", id, "bot-list-id")
	}
	if f.ensureCalls != 1 {
		t.Fatalf("expected EnsureTaskList to be called once, got %d", f.ensureCalls)
	}
	if f.ensureTitle != gtasks.BotTaskListName {
		t.Fatalf("ensureTitle = %q, want %q", f.ensureTitle, gtasks.BotTaskListName)
	}
}

func TestResolveTaskList_EnsureError(t *testing.T) {
	f := &fakeAPI{ensureErr: errors.New("boom")}
	_, err := gtasks.ResolveTaskList(context.Background(), f)
	if err == nil {
		t.Fatal("expected error from EnsureTaskList to propagate")
	}
}
