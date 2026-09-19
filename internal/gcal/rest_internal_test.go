package gcal

import "testing"

func TestToEventTime(t *testing.T) {
	tests := []struct {
		name     string
		dt       string
		tz       string
		wantDate string
		wantDT   string
		wantTZ   string
	}{
		{
			name:     "date-only defaults to all-day event",
			dt:       "2026-09-20",
			wantDate: "2026-09-20",
		},
		{
			name:   "date-time defaults timezone to Asia/Manila",
			dt:     "2026-09-20T10:00:00",
			wantDT: "2026-09-20T10:00:00",
			wantTZ: DefaultTimeZone,
		},
		{
			name:   "explicit timezone is preserved",
			dt:     "2026-09-20T10:00:00",
			tz:     "America/New_York",
			wantDT: "2026-09-20T10:00:00",
			wantTZ: "America/New_York",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toEventTime(tt.dt, tt.tz)
			if got.Date != tt.wantDate || got.DateTime != tt.wantDT || got.TimeZone != tt.wantTZ {
				t.Fatalf("toEventTime(%q, %q) = %+v, want date=%q dateTime=%q timeZone=%q",
					tt.dt, tt.tz, got, tt.wantDate, tt.wantDT, tt.wantTZ)
			}
		})
	}
}
