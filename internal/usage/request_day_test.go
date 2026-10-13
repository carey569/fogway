package usage

import (
	"testing"
	"time"

	"github.com/fogway/fogway/internal/testenv"
)

func TestDayFilterDST(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	testenv.Zone(t, loc)
	for _, day := range []string{"2026-03-08", "2026-11-01"} {
		start, err := time.ParseInLocation(time.DateOnly, day, loc)
		if err != nil {
			t.Fatal(err)
		}
		end := start.AddDate(0, 0, 1)
		f := Filter{Day: day}
		for _, at := range []time.Time{start, start.Add(time.Hour), end.Add(-time.Nanosecond)} {
			if !f.keeps(Record{Time: at.UTC()}) {
				t.Fatalf("%s lost %v", day, at)
			}
		}
		if f.keeps(Record{Time: start.Add(-time.Nanosecond)}) || f.keeps(Record{Time: end}) {
			t.Fatalf("%s included another day", day)
		}
	}
}
