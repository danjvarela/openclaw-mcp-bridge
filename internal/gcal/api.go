package gcal

import "context"

// BotCalendarName is the summary (display name) of the dedicated calendar
// that create_event/update_event/delete_event are restricted to. list_events
// may read any calendar.
const BotCalendarName = "Bot"

// DefaultTimeZone is applied to event start/end times when the caller
// doesn't specify one, per this deployment's Asia/Manila convention.
const DefaultTimeZone = "Asia/Manila"

// CalendarListEntry is one entry from the user's calendar list.
type CalendarListEntry struct {
	ID      string
	Summary string
}

// EventInput is the caller-supplied shape for creating/updating an event.
// Start/End are either an RFC3339 date-time (e.g. "2026-09-20T10:00:00+08:00"
// or without offset, paired with TimeZone) or a bare "YYYY-MM-DD" date for an
// all-day event.
type EventInput struct {
	Summary     string
	Description string
	Location    string
	Start       string
	End         string
	TimeZone    string
}

// Event is the subset of a Google Calendar event this bridge surfaces back
// to the caller.
type Event struct {
	ID          string
	Summary     string
	Description string
	Location    string
	Start       string
	End         string
	TimeZone    string
	HTMLLink    string
}

// ListEventsOptions filters a list_events call.
type ListEventsOptions struct {
	TimeMin    string // RFC3339, inclusive lower bound
	TimeMax    string // RFC3339, exclusive upper bound
	MaxResults int    // 0 means API default
	Query      string // free-text search (Google's `q` param)
}

// API is the Google Calendar surface this package needs. It exists so
// permission/resolution logic can be unit tested against a fake without
// hitting the network.
type API interface {
	// ListCalendars returns the user's calendar list (personal + shared +
	// the Bot calendar), used both by list_events' calendar resolution and
	// by EnsureCalendar's existence check.
	ListCalendars(ctx context.Context) ([]CalendarListEntry, error)

	// EnsureCalendar returns the id of the calendar named summary. It does
	// not create the calendar — the bridge's OAuth scope can't call
	// calendars.insert — so summary must already exist (created once by
	// hand); a missing calendar is an error.
	EnsureCalendar(ctx context.Context, summary string) (id string, err error)

	InsertEvent(ctx context.Context, calendarID string, in EventInput) (*Event, error)
	UpdateEvent(ctx context.Context, calendarID, eventID string, in EventInput) (*Event, error)
	DeleteEvent(ctx context.Context, calendarID, eventID string) error
	ListEvents(ctx context.Context, calendarID string, opts ListEventsOptions) ([]Event, error)
}
