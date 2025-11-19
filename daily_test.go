package main

import (
	"strconv"
	"testing"
	"time"

	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/theHilikus/daily/internal/ui"
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

	for i, test2 := range currentEvents {
		duration, err := time.ParseDuration(test2.originalDuration)
		if err != nil {
			t.Fatal("Error parsing original test duration " + strconv.Itoa(i))
		}
		if actual := createUserFriendlyDurationText(duration); actual != test2.expectedString {
			t.Errorf("%d. Actual %q doesn't match expected %q. Original was %q", i, actual, test2.expectedString, test2.originalDuration)
		}
	}
}

func TestProcessEvents_PreservesExpandedState(t *testing.T) {
	// Setup
	app := test.NewApp()
	defer app.Quit()
	dailyApp = app // Set global dailyApp for functions that use it

	window := test.NewWindow(nil)
	defer window.Close()

	eventsList = container.NewVBox()
	window.SetContent(eventsList)

	now := time.Now()
	testEvents := []event{
		{id: "event1", title: "Event 1", start: now, end: now.Add(time.Hour), details: "details1"},
		{id: "event2", title: "Event 2", start: now.Add(time.Hour), end: now.Add(2 * time.Hour), details: "details2"},
	}

	// 1. Initial processing of events
	processEvents(testEvents, make(map[string]bool))
	assert.Len(t, eventsList.Objects, 2, "Should have two event widgets")

	// 2. Simulate expanding an event
	var eventToExpand *ui.Event
	for _, obj := range eventsList.Objects {
		if ev, ok := obj.(*ui.Event); ok && ev.Id == "event2" {
			eventToExpand = ev
			break
		}
	}
	require.NotNil(t, eventToExpand, "Event widget with ID 'event2' should be found")

	eventToExpand.Open()
	assert.True(t, eventToExpand.IsOpen(), "Event should be open after calling Open()")

	// 3. Simulate a refresh operation
	// 3a. Save the expanded state from the UI
	expandedStates := getExpandedStates()
	assert.Contains(t, expandedStates, "event2", "Expanded state for event2 should be saved")

	eventsList.RemoveAll()

	// 3c. Repopulate the list using the saved state
	processEvents(testEvents, expandedStates)
	assert.Len(t, eventsList.Objects, 2, "Should have two event widgets after refresh")

	// 4. Verify the state is preserved in the new UI objects
	var refreshedEvent *ui.Event
	for _, obj := range eventsList.Objects {
		if ev, ok := obj.(*ui.Event); ok && ev.Id == "event2" {
			refreshedEvent = ev
			break
		}
	}
	require.NotNil(t, refreshedEvent, "Refreshed event widget with ID 'event2' should be found")
	assert.True(t, refreshedEvent.IsOpen(), "Event should remain expanded after refresh")
}
