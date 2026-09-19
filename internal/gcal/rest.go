package gcal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// baseURL is the Calendar API v3 REST root. Overridable in tests.
const baseURL = "https://www.googleapis.com/calendar/v3"

// restAPI implements API over the Calendar v3 REST API directly with
// net/http, rather than pulling in google.golang.org/api/calendar/v3: that
// client library's dependency tree is heavy for a small static stdio bridge,
// and this package needs only six endpoints.
type restAPI struct {
	client  *http.Client
	baseURL string
}

// NewRESTAPI returns an API backed by the Calendar v3 REST API, using
// client (expected to be an OAuth2-wrapped client from NewHTTPClient) for
// auth and transport.
func NewRESTAPI(client *http.Client) API {
	return &restAPI{client: client, baseURL: baseURL}
}

type calendarListResponse struct {
	Items []struct {
		ID      string `json:"id"`
		Summary string `json:"summary"`
	} `json:"items"`
}

func (r *restAPI) ListCalendars(ctx context.Context) ([]CalendarListEntry, error) {
	var out calendarListResponse
	if err := r.do(ctx, http.MethodGet, "/users/me/calendarList", nil, &out); err != nil {
		return nil, err
	}
	entries := make([]CalendarListEntry, 0, len(out.Items))
	for _, it := range out.Items {
		entries = append(entries, CalendarListEntry{ID: it.ID, Summary: it.Summary})
	}
	return entries, nil
}

func (r *restAPI) EnsureCalendar(ctx context.Context, summary string) (string, error) {
	entries, err := r.ListCalendars(ctx)
	if err != nil {
		return "", err
	}
	if entry, found := findCalendar(entries, summary); found {
		return entry.ID, nil
	}

	body := map[string]string{
		"summary":  summary,
		"timeZone": DefaultTimeZone,
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := r.do(ctx, http.MethodPost, "/calendars", body, &out); err != nil {
		return "", fmt.Errorf("gcal: create calendar %q: %w", summary, err)
	}
	return out.ID, nil
}

type eventTime struct {
	Date     string `json:"date,omitempty"`
	DateTime string `json:"dateTime,omitempty"`
	TimeZone string `json:"timeZone,omitempty"`
}

type eventBody struct {
	Summary     string    `json:"summary,omitempty"`
	Description string    `json:"description,omitempty"`
	Location    string    `json:"location,omitempty"`
	Start       eventTime `json:"start"`
	End         eventTime `json:"end"`
}

type eventResponse struct {
	ID          string    `json:"id"`
	Summary     string    `json:"summary"`
	Description string    `json:"description"`
	Location    string    `json:"location"`
	Start       eventTime `json:"start"`
	End         eventTime `json:"end"`
	HTMLLink    string    `json:"htmlLink"`
}

func toEventTime(dt, tz string) eventTime {
	if tz == "" {
		tz = DefaultTimeZone
	}
	if isDateOnly(dt) {
		return eventTime{Date: dt}
	}
	return eventTime{DateTime: dt, TimeZone: tz}
}

// isDateOnly reports whether dt looks like a bare "YYYY-MM-DD" date rather
// than a date-time.
func isDateOnly(dt string) bool {
	return len(dt) == 10 && !strings.Contains(dt, "T")
}

func toEvent(er eventResponse) *Event {
	return &Event{
		ID:          er.ID,
		Summary:     er.Summary,
		Description: er.Description,
		Location:    er.Location,
		Start:       firstNonEmpty(er.Start.DateTime, er.Start.Date),
		End:         firstNonEmpty(er.End.DateTime, er.End.Date),
		TimeZone:    er.Start.TimeZone,
		HTMLLink:    er.HTMLLink,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func (r *restAPI) InsertEvent(ctx context.Context, calendarID string, in EventInput) (*Event, error) {
	body := eventBody{
		Summary:     in.Summary,
		Description: in.Description,
		Location:    in.Location,
		Start:       toEventTime(in.Start, in.TimeZone),
		End:         toEventTime(in.End, in.TimeZone),
	}
	var out eventResponse
	path := fmt.Sprintf("/calendars/%s/events", url.PathEscape(calendarID))
	if err := r.do(ctx, http.MethodPost, path, body, &out); err != nil {
		return nil, fmt.Errorf("gcal: create event: %w", err)
	}
	return toEvent(out), nil
}

func (r *restAPI) UpdateEvent(ctx context.Context, calendarID, eventID string, in EventInput) (*Event, error) {
	body := eventBody{
		Summary:     in.Summary,
		Description: in.Description,
		Location:    in.Location,
		Start:       toEventTime(in.Start, in.TimeZone),
		End:         toEventTime(in.End, in.TimeZone),
	}
	var out eventResponse
	path := fmt.Sprintf("/calendars/%s/events/%s", url.PathEscape(calendarID), url.PathEscape(eventID))
	if err := r.do(ctx, http.MethodPut, path, body, &out); err != nil {
		return nil, fmt.Errorf("gcal: update event: %w", err)
	}
	return toEvent(out), nil
}

func (r *restAPI) DeleteEvent(ctx context.Context, calendarID, eventID string) error {
	path := fmt.Sprintf("/calendars/%s/events/%s", url.PathEscape(calendarID), url.PathEscape(eventID))
	if err := r.do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("gcal: delete event: %w", err)
	}
	return nil
}

type listEventsResponse struct {
	Items []eventResponse `json:"items"`
}

func (r *restAPI) ListEvents(ctx context.Context, calendarID string, opts ListEventsOptions) ([]Event, error) {
	q := url.Values{}
	q.Set("singleEvents", "true")
	q.Set("orderBy", "startTime")
	if opts.TimeMin != "" {
		q.Set("timeMin", opts.TimeMin)
	}
	if opts.TimeMax != "" {
		q.Set("timeMax", opts.TimeMax)
	}
	if opts.MaxResults > 0 {
		q.Set("maxResults", strconv.Itoa(opts.MaxResults))
	}
	if opts.Query != "" {
		q.Set("q", opts.Query)
	}

	path := fmt.Sprintf("/calendars/%s/events?%s", url.PathEscape(calendarID), q.Encode())
	var out listEventsResponse
	if err := r.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, fmt.Errorf("gcal: list events: %w", err)
	}
	events := make([]Event, 0, len(out.Items))
	for _, it := range out.Items {
		events = append(events, *toEvent(it))
	}
	return events, nil
}

// do issues an HTTP request against the Calendar API and decodes a JSON
// response into out (skipped if out is nil, e.g. for a 204 delete).
func (r *restAPI) do(ctx context.Context, method, path string, body, out any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("gcal: encode request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, r.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("gcal: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("gcal: request %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("gcal: read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gcal: %s %s: %s: %s", method, path, resp.Status, truncate(string(respBody), 500))
	}
	if out == nil || len(respBody) == 0 {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("gcal: decode response: %w", err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
