package main

import (
	"log/slog"
	"strconv"
	"time"
)

type dummyEventSource struct {
	originalNow time.Time
	yesterday   []event
	today       []event
	tomorrow    []event
}

func newDummyEventSource() EventSource {
	now := time.Now().Truncate(time.Minute)
	start1 := now.Add(-3 * time.Hour)
	end1 := start1.Add(30 * time.Minute)
	return &dummyEventSource{
		originalNow: now,
		yesterday: []event{
			{id: "1", title: "past event yesterday with zoom", location: "http://www.zoom.us/1234", details: "Past event", start: start1.Add(-24 * time.Hour), end: time.Now().Add(-24*time.Hour + 30*time.Minute)},
		},
		today: []event{
			{id: "2", title: "past event", location: "location1", details: "details1", start: start1, end: end1, response: accepted},
			{id: "3", title: "past event with zoom meeting", location: "http://www.zoom.us/1234", details: "detauls2", start: start1.Add(time.Hour), end: end1.Add(time.Hour), response: declined},
			{id: "4", title: "current event", location: "location3", details: "detauls3 with link https://example.org/go", start: now.Add(-10 * time.Minute), end: now.Add(30 * time.Minute), response: declined, recurring: true},
			{id: "5", title: "A very long current event with zoom meeting that is longer than the rest", location: "https://www.zoom.us/2345", details: "details4 <a href='https://example.com/go'>https://example.com/go</a>", start: now, end: now.Add(time.Minute), response: tentative},
			{id: "6", title: "future event today with html details & others", location: "location5", details: "<p><br>──────────<br><br>Join Zoom Meeting<br>https://www.zoom.us/j/1111?pwd=11111<br>Meeting ID: 111<br>  Password: 11111<br>Phone number :US: +1 564 217 2000<br><br>One touch:8# US<br><br>Find your local number: https://www.zoom.us/u/acsyqrWx7k<br><br><br>──────────</p>", start: now.Add(1 * time.Minute), end: time.Now().Add(6*time.Hour + 30*time.Minute), response: needsAction},
			{id: "7", title: "future event today with gmeeting", location: "https://meet.google.com/3456?a=33&b=66", details: "An event in Google Meeting", start: now.Add(2 * time.Minute), end: time.Now().Add(7*time.Hour + 30*time.Minute), notifiable: true, response: accepted},
		},
		tomorrow: []event{
			{id: "8", title: "future event tomorrow with gmeeting", location: "https://meet.google.com/3456", details: "Future Event", start: start1.Add(24 * time.Hour), end: time.Now().Add(24*time.Hour + 30*time.Minute)},
		},
	}
}

func newDummyEventSourceWithTodayEvents(todayEvents []event) EventSource {
	now := time.Now().Truncate(time.Minute)
	return &dummyEventSource{
		originalNow: now,
		today:       todayEvents}
}

func (dummy dummyEventSource) getDayEvents(day time.Time, fullRefresh bool) ([]event, error) {
	slog.Debug("Returning dummy events. Full refresh = " + strconv.FormatBool(fullRefresh))

	var result []event
	if isOnSameDay(dummy.originalNow, day) {
		result = dummy.today
	} else if day.Before(dummy.originalNow) {
		//past
		result = dummy.yesterday
	} else {
		//future
		result = dummy.tomorrow
	}

	return result, nil
}
