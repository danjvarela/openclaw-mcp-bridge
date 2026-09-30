// Package gtasks implements the Google Tasks tools exposed by the bridge:
// create_task, update_task, delete_task, complete_task, list_tasks, all
// restricted to a single fixed default task list ("Bot"). Auth is an
// OAuth2 refresh-token flow against Google's endpoint, shared plumbing from
// internal/googleoauth.
package gtasks

import "context"

// BotTaskListName is the title of the dedicated task list every tool in
// this package operates on.
const BotTaskListName = "Bot"

// TaskInput is the caller-supplied shape for creating/updating a task.
// Due, if set, is a bare "YYYY-MM-DD" date: the Tasks API only honors the
// date portion of a task's due timestamp.
type TaskInput struct {
	Title string
	Notes string
	Due   string
}

// Task is the subset of a Google Tasks task this bridge surfaces back to
// the caller.
type Task struct {
	ID        string
	Title     string
	Notes     string
	Due       string
	Status    string // "needsAction" or "completed"
	Completed bool
}

// ListTasksOptions filters a list_tasks call.
type ListTasksOptions struct {
	ShowCompleted bool
}

// API is the Google Tasks surface this package needs. It exists so
// resolution logic can be unit tested against a fake without hitting the
// network.
type API interface {
	// ListTaskLists returns the user's task lists.
	ListTaskLists(ctx context.Context) ([]TaskList, error)

	// EnsureTaskList returns the id of the task list titled title,
	// creating it if it doesn't already exist. Unlike gcal's
	// EnsureCalendar, the Tasks API's `tasks` scope does permit
	// tasklists.insert, so this package doesn't require the human to
	// pre-create the list by hand.
	EnsureTaskList(ctx context.Context, title string) (id string, err error)

	InsertTask(ctx context.Context, taskListID string, in TaskInput) (*Task, error)
	UpdateTask(ctx context.Context, taskListID, taskID string, in TaskInput) (*Task, error)
	DeleteTask(ctx context.Context, taskListID, taskID string) error
	ListTasks(ctx context.Context, taskListID string, opts ListTasksOptions) ([]Task, error)
	CompleteTask(ctx context.Context, taskListID, taskID string) (*Task, error)
}

// TaskList is one entry from the user's task list collection.
type TaskList struct {
	ID    string
	Title string
}
