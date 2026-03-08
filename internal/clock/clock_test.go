package clock

import (
	"testing"
	"time"
)

type fixedClock struct{ t time.Time }

func (f fixedClock) Now() time.Time { return f.t }

func TestBusinessDateCT_CutoffAndWeekendRollForward(t *testing.T) {
	loc, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		now  string
		want string
	}{
		{"before cutoff weekday", "2024-01-10T14:59:00-06:00", "2024-01-10"},
		{"at cutoff weekday", "2024-01-10T15:00:00-06:00", "2024-01-11"},
		{"after cutoff weekday", "2024-01-10T15:01:00-06:00", "2024-01-11"},
		{"friday after cutoff", "2024-01-12T15:01:00-06:00", "2024-01-15"},
		{"saturday morning", "2024-01-13T10:00:00-06:00", "2024-01-13"},
		{"sunday morning", "2024-01-14T10:00:00-06:00", "2024-01-15"},
		{"saturday after cutoff", "2024-01-13T15:01:00-06:00", "2024-01-15"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now, err := time.Parse(time.RFC3339, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			got := BusinessDateCT(fixedClock{now}, loc, 15, 0)
			if got != tc.want {
				t.Errorf("BusinessDateCT(%s) = %s, want %s", tc.now, got, tc.want)
			}
		})
	}
}
