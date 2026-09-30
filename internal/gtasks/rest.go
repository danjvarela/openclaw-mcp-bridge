package gtasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// baseURL is the Tasks API v1 REST root. Overridable in tests.
const baseURL = "https://www.googleapis.com/tasks/v1"

// restAPI implements API over the Tasks v1 REST API directly with
// net/http, rather than pulling in google.golang.org/api/tasks/v1: that
// client library's dependency tree is heavy for a small static stdio
// bridge, and this package needs only a handful of endpoints (mirrors the
// gcal package's convention).
type restAPI struct {
	client  *http.Client
	baseURL string
}

// NewRESTAPI returns an API backed by the Tasks v1 REST API, using client
// (expected to be an OAuth2-wrapped client from googleoauth.NewHTTPClient)
// for auth and transport.
func NewRESTAPI(client *http.Client) API {
	return &restAPI{client: client, baseURL: baseURL}
}

type taskListsResponse struct {
	Items []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"items"`
}

func (r *restAPI) ListTaskLists(ctx context.Context) ([]TaskList, error) {
	var out taskListsResponse
	if err := r.do(ctx, http.MethodGet, "/users/@me/lists", nil, &out); err != nil {
		return nil, fmt.Errorf("gtasks: list task lists: %w", err)
	}
	lists := make([]TaskList, 0, len(out.Items))
	for _, it := range out.Items {
		lists = append(lists, TaskList{ID: it.ID, Title: it.Title})
	}
	return lists, nil
}

type taskListBody struct {
	Title string `json:"title"`
}

type taskListResponse struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// EnsureTaskList looks up the task list titled title, creating it if it
// doesn't already exist.
func (r *restAPI) EnsureTaskList(ctx context.Context, title string) (string, error) {
	lists, err := r.ListTaskLists(ctx)
	if err != nil {
		return "", err
	}
	if list, found := findTaskList(lists, title); found {
		return list.ID, nil
	}

	var out taskListResponse
	if err := r.do(ctx, http.MethodPost, "/users/@me/lists", taskListBody{Title: title}, &out); err != nil {
		return "", fmt.Errorf("gtasks: create task list %q: %w", title, err)
	}
	return out.ID, nil
}

func findTaskList(lists []TaskList, title string) (TaskList, bool) {
	for _, l := range lists {
		if l.Title == title {
			return l, true
		}
	}
	return TaskList{}, false
}

type taskBody struct {
	Title  string `json:"title,omitempty"`
	Notes  string `json:"notes,omitempty"`
	Due    string `json:"due,omitempty"`
	Status string `json:"status,omitempty"`
}

type taskResponse struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Notes  string `json:"notes"`
	Due    string `json:"due"`
	Status string `json:"status"`
}

func toTask(tr taskResponse) *Task {
	return &Task{
		ID:        tr.ID,
		Title:     tr.Title,
		Notes:     tr.Notes,
		Due:       tr.Due,
		Status:    tr.Status,
		Completed: tr.Status == "completed",
	}
}

func toTaskBody(in TaskInput) taskBody {
	due := ""
	if in.Due != "" {
		due = in.Due + "T00:00:00.000Z"
	}
	return taskBody{Title: in.Title, Notes: in.Notes, Due: due}
}

func (r *restAPI) InsertTask(ctx context.Context, taskListID string, in TaskInput) (*Task, error) {
	var out taskResponse
	path := fmt.Sprintf("/lists/%s/tasks", url.PathEscape(taskListID))
	if err := r.do(ctx, http.MethodPost, path, toTaskBody(in), &out); err != nil {
		return nil, fmt.Errorf("gtasks: create task: %w", err)
	}
	return toTask(out), nil
}

func (r *restAPI) UpdateTask(ctx context.Context, taskListID, taskID string, in TaskInput) (*Task, error) {
	var out taskResponse
	path := fmt.Sprintf("/lists/%s/tasks/%s", url.PathEscape(taskListID), url.PathEscape(taskID))
	if err := r.do(ctx, http.MethodPatch, path, toTaskBody(in), &out); err != nil {
		return nil, fmt.Errorf("gtasks: update task: %w", err)
	}
	return toTask(out), nil
}

func (r *restAPI) DeleteTask(ctx context.Context, taskListID, taskID string) error {
	path := fmt.Sprintf("/lists/%s/tasks/%s", url.PathEscape(taskListID), url.PathEscape(taskID))
	if err := r.do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("gtasks: delete task: %w", err)
	}
	return nil
}

func (r *restAPI) CompleteTask(ctx context.Context, taskListID, taskID string) (*Task, error) {
	var out taskResponse
	path := fmt.Sprintf("/lists/%s/tasks/%s", url.PathEscape(taskListID), url.PathEscape(taskID))
	if err := r.do(ctx, http.MethodPatch, path, taskBody{Status: "completed"}, &out); err != nil {
		return nil, fmt.Errorf("gtasks: complete task: %w", err)
	}
	return toTask(out), nil
}

type listTasksResponse struct {
	Items []taskResponse `json:"items"`
}

func (r *restAPI) ListTasks(ctx context.Context, taskListID string, opts ListTasksOptions) ([]Task, error) {
	q := url.Values{}
	if opts.ShowCompleted {
		q.Set("showCompleted", "true")
		q.Set("showHidden", "true")
	} else {
		q.Set("showCompleted", "false")
	}

	path := fmt.Sprintf("/lists/%s/tasks?%s", url.PathEscape(taskListID), q.Encode())
	var out listTasksResponse
	if err := r.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, fmt.Errorf("gtasks: list tasks: %w", err)
	}
	tasks := make([]Task, 0, len(out.Items))
	for _, it := range out.Items {
		tasks = append(tasks, *toTask(it))
	}
	return tasks, nil
}

// do issues an HTTP request against the Tasks API and decodes a JSON
// response into out (skipped if out is nil, e.g. for a 204 delete).
func (r *restAPI) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("gtasks: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, r.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("gtasks: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("gtasks: request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("gtasks: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gtasks: %s %s: %s: %s", method, path, resp.Status, truncate(string(respBody), 500))
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("gtasks: decode response: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
