package main

import (
	"testing"
	"time"
)

func TestElapsed(t *testing.T) {
	for d, want := range map[time.Duration]string{
		850 * time.Millisecond:                "850ms",
		12340 * time.Millisecond:              "12.3s",
		2*time.Minute + 5*time.Second:         "2m05s",
		2*time.Minute + 4500*time.Millisecond: "2m05s",
		61 * time.Minute:                      "61m00s",
	} {
		if got := elapsed(d); got != want {
			t.Errorf("elapsed(%v) = %q, want %q", d, got, want)
		}
	}
}
