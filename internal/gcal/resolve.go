package gcal

import (
	"context"
	"fmt"
	"strings"
)

// ResolveWriteCalendar resolves ref (a calendar id, calendar name, or empty)
// to a calendar id for a write operation (create_event/update_event/
// delete_event), enforcing that the target is the "Bot" calendar. It never
// calls a write endpoint itself.
//
// An empty ref or a case-insensitive match on BotCalendarName resolves to
// the Bot calendar via EnsureCalendar (which requires it to already exist —
// see EnsureCalendar's doc comment). Any other ref is looked up in the
// calendar list; if it matches an entry
// whose Summary is "Bot" (by id or by name) it resolves to that entry's id,
// otherwise resolution fails and no API call that could mutate data is made.
func ResolveWriteCalendar(ctx context.Context, api API, ref string) (string, error) {
	if ref == "" || strings.EqualFold(ref, BotCalendarName) {
		return api.EnsureCalendar(ctx, BotCalendarName)
	}

	entries, err := api.ListCalendars(ctx)
	if err != nil {
		return "", fmt.Errorf("gcal: list calendars: %w", err)
	}

	entry, found := findCalendar(entries, ref)
	if !found {
		return "", fmt.Errorf("gcal: refusing write: calendar %q is not the Bot calendar (unknown calendar; only the %q calendar accepts writes)", ref, BotCalendarName)
	}
	if !strings.EqualFold(entry.Summary, BotCalendarName) {
		return "", fmt.Errorf("gcal: refusing write: calendar %q (%q) is not the Bot calendar; only the %q calendar accepts writes", ref, entry.Summary, BotCalendarName)
	}
	return entry.ID, nil
}

// ResolveReadCalendar resolves ref to a calendar id for list_events, which
// may read any calendar. An empty ref or "primary" resolves to the user's
// primary calendar. Any other ref is matched by id or name against the
// calendar list; an unmatched ref is passed through unchanged (Google
// accepts arbitrary calendar ids, including ones not enumerable via
// calendarList, e.g. some shared calendars).
func ResolveReadCalendar(ctx context.Context, api API, ref string) (string, error) {
	if ref == "" || strings.EqualFold(ref, "primary") {
		return "primary", nil
	}

	entries, err := api.ListCalendars(ctx)
	if err != nil {
		return "", fmt.Errorf("gcal: list calendars: %w", err)
	}
	if entry, found := findCalendar(entries, ref); found {
		return entry.ID, nil
	}
	return ref, nil
}

func findCalendar(entries []CalendarListEntry, ref string) (CalendarListEntry, bool) {
	for _, e := range entries {
		if e.ID == ref || strings.EqualFold(e.Summary, ref) {
			return e, true
		}
	}
	return CalendarListEntry{}, false
}
