package app

import (
	"testing"
	"time"
)

func TestTransitions(t *testing.T) {
	for _, x := range []struct {
		s, a string
		want bool
	}{{"available", "assign", true}, {"assigned", "dispose", false}, {"maintenance", "transfer", false}, {"disposed", "assign", false}, {"assigned", "return", true}} {
		if allowed(x.s, x.a) != x.want {
			t.Fatalf("%s/%s", x.s, x.a)
		}
	}
}
func TestBookValue(t *testing.T) {
	d := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	for _, x := range []struct {
		at   time.Time
		want int64
	}{{d.AddDate(0, 6, 0), 550}, {d.AddDate(0, 12, 0), 100}, {d.AddDate(0, 0, -1), 1000}, {d.AddDate(0, 1, -1), 1000}} {
		if got := bookValue(1000, 100, 12, d, x.at); got != x.want {
			t.Fatalf("got %d want %d", got, x.want)
		}
	}
}
func TestValidation(t *testing.T) {
	if validateMoney(100, 101, 12) == nil || validateMoney(100, 0, 0) == nil || validateMoney(-1, 0, 12) == nil {
		t.Fatal("invalid accepted")
	}
}
