package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/danjvarela/openclaw-mcp-bridge/internal/gcal"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerCalendarTools adds the four Google Calendar tools to server.
// Credentials are read lazily on first tool call (not at startup), so a
// bridge invoked without calendar credentials configured (e.g. only
// sync_notes is needed) still starts cleanly and other tools remain usable;
// only a calendar tool call fails with a clear error.
func registerCalendarTools(server *mcp.Server) {
	server.AddTool(&mcp.Tool{
		Name: "create_event",
		Description: "Create a calendar event on the Bot calendar (the dedicated calendar this bridge is allowed to write to; it is auto-created on first use). " +
			"Refuses if asked to write to any other calendar. Times default to Asia/Manila when timezone is omitted.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"summary": {"type": "string", "description": "Event title."},
				"description": {"type": "string", "description": "Optional longer description."},
				"location": {"type": "string", "description": "Optional location text."},
				"start": {"type": "string", "description": "Start time as RFC3339 date-time (e.g. 2026-09-20T10:00:00) or YYYY-MM-DD for an all-day event."},
				"end": {"type": "string", "description": "End time, same format as start."},
				"timezone": {"type": "string", "description": "IANA timezone, e.g. Asia/Manila (default) or America/New_York. Ignored for all-day events."}
			},
			"required": ["summary", "start", "end"]
		}`),
	}, createEvent)

	server.AddTool(&mcp.Tool{
		Name: "update_event",
		Description: "Update an existing event on the Bot calendar. Refuses if calendar refers to anything other than the Bot calendar. " +
			"All fields except event_id are replaced with the given values (Google's events.update semantics), so pass the full desired event.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"event_id": {"type": "string", "description": "The event's id, as returned by create_event or list_events."},
				"summary": {"type": "string", "description": "Event title."},
				"description": {"type": "string", "description": "Optional longer description."},
				"location": {"type": "string", "description": "Optional location text."},
				"start": {"type": "string", "description": "Start time as RFC3339 date-time or YYYY-MM-DD for all-day."},
				"end": {"type": "string", "description": "End time, same format as start."},
				"timezone": {"type": "string", "description": "IANA timezone, default Asia/Manila."}
			},
			"required": ["event_id", "summary", "start", "end"]
		}`),
	}, updateEvent)

	server.AddTool(&mcp.Tool{
		Name:        "delete_event",
		Description: "Delete an event from the Bot calendar. Refuses if calendar refers to anything other than the Bot calendar.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"event_id": {"type": "string", "description": "The event's id, as returned by create_event or list_events."}
			},
			"required": ["event_id"]
		}`),
	}, deleteEvent)

	server.AddTool(&mcp.Tool{
		Name: "list_events",
		Description: "List events from any of the user's calendars (read-only), including personal calendars, not just the Bot calendar. " +
			"Pass calendar as a calendar id, \"primary\" for the user's main calendar, a calendar display name (e.g. \"Bot\", \"Personal\"), or omit for primary.",
		InputSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"calendar": {"type": "string", "description": "Calendar id, display name, or \"primary\". Defaults to primary."},
				"time_min": {"type": "string", "description": "RFC3339 lower bound (inclusive), e.g. 2026-09-19T00:00:00+08:00."},
				"time_max": {"type": "string", "description": "RFC3339 upper bound (exclusive)."},
				"query": {"type": "string", "description": "Free-text search over event fields."},
				"max_results": {"type": "integer", "description": "Maximum number of events to return."}
			}
		}`),
	}, listEvents)
}

// newCalendarAPI builds a Calendar API client from the credentials at
// GOOGLE_CALENDAR_CREDENTIALS. Built per call rather than cached at startup:
// the bridge is a short-lived per-turn subprocess (see package doc in
// main.go), so there's no long-lived state worth caching across calls.
func newCalendarAPI(ctx context.Context) (gcal.API, error) {
	path, err := gcal.CredentialsPathFromEnv(os.Getenv)
	if err != nil {
		return nil, err
	}
	creds, err := gcal.LoadCredentials(path)
	if err != nil {
		return nil, err
	}
	client := gcal.NewHTTPClient(ctx, creds)
	return gcal.NewRESTAPI(client), nil
}

type eventArgs struct {
	Calendar    string `json:"calendar"`
	EventID     string `json:"event_id"`
	Summary     string `json:"summary"`
	Description string `json:"description"`
	Location    string `json:"location"`
	Start       string `json:"start"`
	End         string `json:"end"`
	TimeZone    string `json:"timezone"`
}

func parseArgs(req *mcp.CallToolRequest, out any) error {
	if req.Params == nil || len(req.Params.Arguments) == 0 {
		return nil
	}
	if err := json.Unmarshal(req.Params.Arguments, out); err != nil {
		return fmt.Errorf("invalid arguments: %w", err)
	}
	return nil
}

func createEvent(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args eventArgs
	if err := parseArgs(req, &args); err != nil {
		return toolError("create_event: %v", err), nil
	}

	api, err := newCalendarAPI(ctx)
	if err != nil {
		return toolError("create_event not configured: %v", err), nil
	}

	calendarID, err := gcal.ResolveWriteCalendar(ctx, api, args.Calendar)
	if err != nil {
		return toolError("create_event: %v", err), nil
	}

	ev, err := api.InsertEvent(ctx, calendarID, gcal.EventInput{
		Summary:     args.Summary,
		Description: args.Description,
		Location:    args.Location,
		Start:       args.Start,
		End:         args.End,
		TimeZone:    args.TimeZone,
	})
	if err != nil {
		return toolError("create_event failed: %v", err), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: formatEvent(ev)}}}, nil
}

func updateEvent(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args eventArgs
	if err := parseArgs(req, &args); err != nil {
		return toolError("update_event: %v", err), nil
	}
	if args.EventID == "" {
		return toolError("update_event: event_id is required"), nil
	}

	api, err := newCalendarAPI(ctx)
	if err != nil {
		return toolError("update_event not configured: %v", err), nil
	}

	calendarID, err := gcal.ResolveWriteCalendar(ctx, api, args.Calendar)
	if err != nil {
		return toolError("update_event: %v", err), nil
	}

	ev, err := api.UpdateEvent(ctx, calendarID, args.EventID, gcal.EventInput{
		Summary:     args.Summary,
		Description: args.Description,
		Location:    args.Location,
		Start:       args.Start,
		End:         args.End,
		TimeZone:    args.TimeZone,
	})
	if err != nil {
		return toolError("update_event failed: %v", err), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: formatEvent(ev)}}}, nil
}

func deleteEvent(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args eventArgs
	if err := parseArgs(req, &args); err != nil {
		return toolError("delete_event: %v", err), nil
	}
	if args.EventID == "" {
		return toolError("delete_event: event_id is required"), nil
	}

	api, err := newCalendarAPI(ctx)
	if err != nil {
		return toolError("delete_event not configured: %v", err), nil
	}

	calendarID, err := gcal.ResolveWriteCalendar(ctx, api, args.Calendar)
	if err != nil {
		return toolError("delete_event: %v", err), nil
	}

	if err := api.DeleteEvent(ctx, calendarID, args.EventID); err != nil {
		return toolError("delete_event failed: %v", err), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("deleted event %s", args.EventID)}}}, nil
}

type listEventsArgs struct {
	Calendar   string `json:"calendar"`
	TimeMin    string `json:"time_min"`
	TimeMax    string `json:"time_max"`
	Query      string `json:"query"`
	MaxResults int    `json:"max_results"`
}

func listEvents(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var args listEventsArgs
	if err := parseArgs(req, &args); err != nil {
		return toolError("list_events: %v", err), nil
	}

	api, err := newCalendarAPI(ctx)
	if err != nil {
		return toolError("list_events not configured: %v", err), nil
	}

	calendarID, err := gcal.ResolveReadCalendar(ctx, api, args.Calendar)
	if err != nil {
		return toolError("list_events: %v", err), nil
	}

	events, err := api.ListEvents(ctx, calendarID, gcal.ListEventsOptions{
		TimeMin:    args.TimeMin,
		TimeMax:    args.TimeMax,
		Query:      args.Query,
		MaxResults: args.MaxResults,
	})
	if err != nil {
		return toolError("list_events failed: %v", err), nil
	}
	if len(events) == 0 {
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "no events found"}}}, nil
	}

	text := ""
	for i, ev := range events {
		if i > 0 {
			text += "\n"
		}
		text += formatEvent(&ev)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: text}}}, nil
}

func formatEvent(ev *gcal.Event) string {
	return fmt.Sprintf("%s | %s -> %s | %s | id=%s", ev.Summary, ev.Start, ev.End, ev.Location, ev.ID)
}
