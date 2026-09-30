package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/danjvarela/openclaw-mcp-bridge/internal/gtasks"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerTaskTools adds the five Google Tasks tools to server. Credentials
// are read lazily on first tool call, mirroring registerCalendarTools: a
// bridge invoked without task credentials configured still starts cleanly.
func registerTaskTools(server *mcp.Server) {
	server.AddTool(&mcp.Tool{
		Name:        "create_task",
		Description: "Create a task on the Bot task list (the dedicated list this bridge is allowed to write to; auto-created on first use).",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"title": {"type": "string", "description": "Task title."},
				"notes": {"type": "string", "description": "Optional longer description."},
				"due": {"type": "string", "description": "Optional due date as YYYY-MM-DD."}
			},
			"required": ["title"]
		}`),
	}, createTask)

	server.AddTool(&mcp.Tool{
		Name:        "update_task",
		Description: "Update an existing task on the Bot task list. All fields except task_id are replaced with the given values.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"task_id": {"type": "string", "description": "The task's id, as returned by create_task or list_tasks."},
				"title": {"type": "string", "description": "Task title."},
				"notes": {"type": "string", "description": "Optional longer description."},
				"due": {"type": "string", "description": "Optional due date as YYYY-MM-DD."}
			},
			"required": ["task_id", "title"]
		}`),
	}, updateTask)

	server.AddTool(&mcp.Tool{
		Name:        "delete_task",
		Description: "Delete a task from the Bot task list.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"task_id": {"type": "string", "description": "The task's id, as returned by create_task or list_tasks."}
			},
			"required": ["task_id"]
		}`),
	}, deleteTask)

	server.AddTool(&mcp.Tool{
		Name:        "complete_task",
		Description: "Mark a task on the Bot task list as completed.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"task_id": {"type": "string", "description": "The task's id, as returned by create_task or list_tasks."}
			},
			"required": ["task_id"]
		}`),
	}, completeTask)

	server.AddTool(&mcp.Tool{
		Name:        "list_tasks",
		Description: "List tasks on the Bot task list. By default only shows incomplete tasks.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"show_completed": {"type": "boolean", "description": "Include completed tasks. Defaults to false."}
			}
		}`),
	}, listTasks)
}

// newTaskAPI builds a Tasks API client from the credentials at
// GOOGLE_TASKS_CREDENTIALS. Built per call rather than cached at startup,
// mirroring newCalendarAPI: the bridge is a short-lived per-turn subprocess.
func newTaskAPI(ctx context.Context) (gtasks.API, error) {
	path, err := gtasks.CredentialsPathFromEnv(os.Getenv)
	if err != nil {
		return nil, err
	}
	creds, err := gtasks.LoadCredentials(path)
	if err != nil {
		return nil, err
	}
	client := gtasks.NewHTTPClient(ctx, creds)
	return gtasks.NewRESTAPI(client), nil
}

type taskArgs struct {
	TaskID string `json:"task_id"`
	Title  string `json:"title"`
	Notes  string `json:"notes"`
	Due    string `json:"due"`
}

func createTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args taskArgs
	if err := parseArgs(req, &args); err != nil {
		return toolError("create_task: %v", err), nil
	}

	api, err := newTaskAPI(ctx)
	if err != nil {
		return toolError("create_task not configured: %v", err), nil
	}

	taskListID, err := gtasks.ResolveTaskList(ctx, api)
	if err != nil {
		return toolError("create_task: %v", err), nil
	}

	task, err := api.InsertTask(ctx, taskListID, gtasks.TaskInput{
		Title: args.Title,
		Notes: args.Notes,
		Due:   args.Due,
	})
	if err != nil {
		return toolError("create_task failed: %v", err), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: formatTask(task)}}}, nil
}

func updateTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args taskArgs
	if err := parseArgs(req, &args); err != nil {
		return toolError("update_task: %v", err), nil
	}
	if args.TaskID == "" {
		return toolError("update_task: task_id is required"), nil
	}

	api, err := newTaskAPI(ctx)
	if err != nil {
		return toolError("update_task not configured: %v", err), nil
	}

	taskListID, err := gtasks.ResolveTaskList(ctx, api)
	if err != nil {
		return toolError("update_task: %v", err), nil
	}

	task, err := api.UpdateTask(ctx, taskListID, args.TaskID, gtasks.TaskInput{
		Title: args.Title,
		Notes: args.Notes,
		Due:   args.Due,
	})
	if err != nil {
		return toolError("update_task failed: %v", err), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: formatTask(task)}}}, nil
}

func deleteTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args taskArgs
	if err := parseArgs(req, &args); err != nil {
		return toolError("delete_task: %v", err), nil
	}
	if args.TaskID == "" {
		return toolError("delete_task: task_id is required"), nil
	}

	api, err := newTaskAPI(ctx)
	if err != nil {
		return toolError("delete_task not configured: %v", err), nil
	}

	taskListID, err := gtasks.ResolveTaskList(ctx, api)
	if err != nil {
		return toolError("delete_task: %v", err), nil
	}

	if err := api.DeleteTask(ctx, taskListID, args.TaskID); err != nil {
		return toolError("delete_task failed: %v", err), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("deleted task %s", args.TaskID)}}}, nil
}

func completeTask(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args taskArgs
	if err := parseArgs(req, &args); err != nil {
		return toolError("complete_task: %v", err), nil
	}
	if args.TaskID == "" {
		return toolError("complete_task: task_id is required"), nil
	}

	api, err := newTaskAPI(ctx)
	if err != nil {
		return toolError("complete_task not configured: %v", err), nil
	}

	taskListID, err := gtasks.ResolveTaskList(ctx, api)
	if err != nil {
		return toolError("complete_task: %v", err), nil
	}

	task, err := api.CompleteTask(ctx, taskListID, args.TaskID)
	if err != nil {
		return toolError("complete_task failed: %v", err), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: formatTask(task)}}}, nil
}

type listTasksArgs struct {
	ShowCompleted bool `json:"show_completed"`
}

func listTasks(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args listTasksArgs
	if err := parseArgs(req, &args); err != nil {
		return toolError("list_tasks: %v", err), nil
	}

	api, err := newTaskAPI(ctx)
	if err != nil {
		return toolError("list_tasks not configured: %v", err), nil
	}

	taskListID, err := gtasks.ResolveTaskList(ctx, api)
	if err != nil {
		return toolError("list_tasks: %v", err), nil
	}

	tasks, err := api.ListTasks(ctx, taskListID, gtasks.ListTasksOptions{ShowCompleted: args.ShowCompleted})
	if err != nil {
		return toolError("list_tasks failed: %v", err), nil
	}
	if len(tasks) == 0 {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "no tasks found"}}}, nil
	}

	text := ""
	for i, tk := range tasks {
		if i > 0 {
			text += "\n"
		}
		text += formatTask(&tk)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
}

func formatTask(tk *gtasks.Task) string {
	due := tk.Due
	if due == "" {
		due = "no due date"
	}
	status := "incomplete"
	if tk.Completed {
		status = "completed"
	}
	return fmt.Sprintf("%s | %s | %s | id=%s", tk.Title, due, status, tk.ID)
}
