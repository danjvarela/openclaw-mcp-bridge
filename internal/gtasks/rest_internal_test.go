package gtasks

import "testing"

func TestToTaskBody(t *testing.T) {
	tests := []struct {
		name    string
		in      TaskInput
		wantDue string
	}{
		{
			name:    "no due date",
			in:      TaskInput{Title: "buy milk"},
			wantDue: "",
		},
		{
			name:    "due date gets midnight UTC time appended",
			in:      TaskInput{Title: "buy milk", Due: "2026-09-20"},
			wantDue: "2026-09-20T00:00:00.000Z",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toTaskBody(tt.in)
			if got.Due != tt.wantDue {
				t.Fatalf("toTaskBody(%+v).Due = %q, want %q", tt.in, got.Due, tt.wantDue)
			}
			if got.Title != tt.in.Title {
				t.Fatalf("toTaskBody(%+v).Title = %q, want %q", tt.in, got.Title, tt.in.Title)
			}
		})
	}
}
