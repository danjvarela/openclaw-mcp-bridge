package gcal_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/danjvarela/openclaw-mcp-bridge/internal/gcal"
)

// fakeAPI is a network-free stand-in for gcal.API used to test the
// permission/resolution logic in resolve.go.
type fakeAPI struct {
	calendars    []gcal.CalendarListEntry
	ensureCalls  int
	ensureID     string
	ensureErr    error
	listErr      error
	insertCalled bool
	updateCalled bool
	deleteCalled bool
}

func (f *fakeAPI) ListCalendars(context.Context) ([]gcal.CalendarListEntry, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.calendars, nil
}

func (f *fakeAPI) EnsureCalendar(context.Context, string) (string, error) {
	f.ensureCalls++
	if f.ensureErr != nil {
		return "", f.ensureErr
	}
	return f.ensureID, nil
}

func (f *fakeAPI) InsertEvent(context.Context, string, gcal.EventInput) (*gcal.Event, error) {
	f.insertCalled = true
	return &gcal.Event{}, nil
}

func (f *fakeAPI) UpdateEvent(context.Context, string, string, gcal.EventInput) (*gcal.Event, error) {
	f.updateCalled = true
	return &gcal.Event{}, nil
}

func (f *fakeAPI) DeleteEvent(context.Context, string, string) error {
	f.deleteCalled = true
	return nil
}

func (f *fakeAPI) ListEvents(context.Context, string, gcal.ListEventsOptions) ([]gcal.Event, error) {
	return nil, nil
}

func TestResolveWriteCalendar(t *testing.T) {
	tests := []struct {
		name       string
		ref        string
		calendars  []gcal.CalendarListEntry
		ensureID   string
		wantID     string
		wantErr    bool
		wantEnsure bool
	}{
		{
			name:       "empty ref resolves to Bot via ensure",
			ref:        "",
			ensureID:   "bot-id-1",
			wantID:     "bot-id-1",
			wantEnsure: true,
		},
		{
			name:       "case-insensitive bot name resolves via ensure",
			ref:        "bOt",
			ensureID:   "bot-id-1",
			wantID:     "bot-id-1",
			wantEnsure: true,
		},
		{
			name: "ref matching Bot's id in calendar list is allowed",
			ref:  "bot-id-2",
			calendars: []gcal.CalendarListEntry{
				{ID: "personal-id", Summary: "Personal"},
				{ID: "bot-id-2", Summary: "Bot"},
			},
			wantID: "bot-id-2",
		},
		{
			name: "ref matching Bot's name in calendar list is allowed",
			ref:  "Bot",
			calendars: []gcal.CalendarListEntry{
				{ID: "bot-id-3", Summary: "Bot"},
			},
			wantEnsure: true,
			ensureID:   "bot-id-3",
			wantID:     "bot-id-3",
		},
		{
			name: "ref resolving to a non-Bot calendar is refused",
			ref:  "Personal",
			calendars: []gcal.CalendarListEntry{
				{ID: "personal-id", Summary: "Personal"},
				{ID: "bot-id", Summary: "Bot"},
			},
			wantErr: true,
		},
		{
			name: "ref not found in calendar list is refused",
			ref:  "unknown-calendar",
			calendars: []gcal.CalendarListEntry{
				{ID: "bot-id", Summary: "Bot"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAPI{calendars: tt.calendars, ensureID: tt.ensureID}
			id, err := gcal.ResolveWriteCalendar(context.Background(), f, tt.ref)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got id %q", id)
				}
				if f.insertCalled || f.updateCalled || f.deleteCalled {
					t.Fatalf("refused resolution must not call any write API")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if id != tt.wantID {
				t.Fatalf("id = %q, want %q", id, tt.wantID)
			}
			if tt.wantEnsure && f.ensureCalls == 0 {
				t.Fatalf("expected EnsureCalendar to be called")
			}
		})
	}
}

func TestResolveWriteCalendar_EnsureError(t *testing.T) {
	f := &fakeAPI{ensureErr: errors.New("boom")}
	_, err := gcal.ResolveWriteCalendar(context.Background(), f, "")
	if err == nil {
		t.Fatal("expected error from EnsureCalendar to propagate")
	}
}

func TestResolveWriteCalendar_ListError(t *testing.T) {
	f := &fakeAPI{listErr: errors.New("boom")}
	_, err := gcal.ResolveWriteCalendar(context.Background(), f, "some-ref")
	if err == nil {
		t.Fatal("expected list error to propagate")
	}
}

func TestResolveReadCalendar(t *testing.T) {
	tests := []struct {
		name      string
		ref       string
		calendars []gcal.CalendarListEntry
		want      string
	}{
		{name: "empty ref is primary", ref: "", want: "primary"},
		{name: "explicit primary", ref: "primary", want: "primary"},
		{name: "case-insensitive primary", ref: "Primary", want: "primary"},
		{
			name: "resolves by name",
			ref:  "Personal",
			calendars: []gcal.CalendarListEntry{
				{ID: "personal-id", Summary: "Personal"},
			},
			want: "personal-id",
		},
		{
			name: "resolves by id",
			ref:  "personal-id",
			calendars: []gcal.CalendarListEntry{
				{ID: "personal-id", Summary: "Personal"},
			},
			want: "personal-id",
		},
		{
			name: "unmatched ref passes through unchanged",
			ref:  "some-external-calendar-id@group.calendar.google.com",
			want: "some-external-calendar-id@group.calendar.google.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAPI{calendars: tt.calendars}
			got, err := gcal.ResolveReadCalendar(context.Background(), f, tt.ref)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveReadCalendar_ListError(t *testing.T) {
	f := &fakeAPI{listErr: errors.New("boom")}
	_, err := gcal.ResolveReadCalendar(context.Background(), f, "some-ref")
	if err == nil {
		t.Fatal("expected list error to propagate")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Fatalf("error %q should wrap underlying cause", err)
	}
}
