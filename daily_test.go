package main

import (
	"strconv"
	"testing"
	"time"
)

type durationTest struct {
	originalDuration string
	expectedString   string
}

func TestDurationText(t *testing.T) {
	var currentEvents = []durationTest{
		{"2h00m00s", "2h0m"},
		{"1h59m59s", "2h0m"},
		{"1h59m01s", "2h0m"},
		{"0h59m59s", "1h0m"},
		{"0h59m01s", "1h0m"},
		{"1h00m00s", "1h0m"},
		{"0h01m59s", "2m"},
		{"0h01m01s", "2m"},
		{"0h01m00s", "1m"},
		{"0h00m59s", "1m"},
		{"0h00m01s", "1m"},
	}

	for i, test := range currentEvents {
		duration, err := time.ParseDuration(test.originalDuration)
		if err != nil {
			t.Fatal("Error parsing original test duration " + strconv.Itoa(i))
		}
		if actual := createUserFriendlyDurationText(duration); actual != test.expectedString {
			t.Errorf("%d. Actual %q doesn't match expected %q. Original was %q", i, actual, test.expectedString, test.originalDuration)
		}
	}
}
