package main

import (
	"testing"
)

func TestBuildSchedule_EmptyChores(t *testing.T) {
	s := buildSchedule(nil)
	if len(s) != 0 {
		t.Errorf("expected empty schedule, got %d entries", len(s))
	}
}

func TestBuildSchedule_BoundaryDates(t *testing.T) {
	chores := []Chore{{Name: "Test", Person: "Alice", CadenceWeeks: 1}}
	s := buildSchedule(chores)

	if _, ok := s["2026-01-01"]; !ok {
		t.Error("expected Jan 1 2026 in schedule")
	}
	if _, ok := s["2026-12-31"]; !ok {
		// Dec 31 is not exactly 7-day-multiple from Jan 1, so only check it's present if it falls on a week boundary
		// Jan 1 is Thursday. Dec 31 is Thursday — exactly 52 weeks later. It should appear.
		t.Error("expected Dec 31 2026 in schedule")
	}
}

func TestBuildSchedule_WeeklyCadence(t *testing.T) {
	chores := []Chore{{Name: "Test", Person: "Alice", CadenceWeeks: 1}}
	s := buildSchedule(chores)

	// Jan 1 2026 is a Thursday. Weekly cadence should hit every Thursday.
	// Check a few: Jan 1, Jan 8, Jan 15, Jan 22, Jan 29.
	thursdays := []string{"2026-01-01", "2026-01-08", "2026-01-15", "2026-01-22", "2026-01-29"}
	for _, d := range thursdays {
		if _, ok := s[d]; !ok {
			t.Errorf("expected %s in weekly schedule", d)
		}
	}

	// Non-thursdays should not appear.
	if _, ok := s["2026-01-02"]; ok {
		t.Error("expected 2026-01-02 (Friday) to not appear in weekly schedule")
	}
}

func TestBuildSchedule_HalfWeekCadence(t *testing.T) {
	chores := []Chore{{Name: "Test", Person: "Alice", CadenceWeeks: 0.5}}
	s := buildSchedule(chores)

	// 0.5 week = 3.5 days — should appear roughly twice per week (Mon+Thu or similar).
	// Jan 1 (Thu) → Jan 4 (Sun) → Jan 7 (Wed) → Jan 11 (Sun)... verify count is ~104 for the year.
	count := 0
	for range s {
		count++
	}
	if count < 100 || count > 108 {
		t.Errorf("expected ~104 entries for 0.5-week cadence, got %d", count)
	}
}

func TestBuildSchedule_FourWeekCadence(t *testing.T) {
	chores := []Chore{{Name: "Test", Person: "Alice", CadenceWeeks: 4}}
	s := buildSchedule(chores)

	// 4-week cadence over 52 weeks ≈ 13 occurrences.
	count := 0
	for range s {
		count++
	}
	if count < 12 || count > 14 {
		t.Errorf("expected ~13 entries for 4-week cadence, got %d", count)
	}
}

func TestBuildSchedule_SortOrder(t *testing.T) {
	chores := []Chore{
		{Name: "Zebra chore", Person: "Kevina", CadenceWeeks: 1},
		{Name: "Alpha chore", Person: "Alison", CadenceWeeks: 1},
		{Name: "Beta chore", Person: "Alison", CadenceWeeks: 1},
	}
	s := buildSchedule(chores)

	entries, ok := s["2026-01-01"]
	if !ok {
		t.Fatal("expected 2026-01-01 in schedule")
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries on 2026-01-01, got %d", len(entries))
	}
	// Sorted by person then name: Alison Alpha, Alison Beta, Kevina Zebra
	if entries[0].Person != "Alison" || entries[0].Name != "Alpha chore" {
		t.Errorf("expected first entry Alison/Alpha chore, got %s/%s", entries[0].Person, entries[0].Name)
	}
	if entries[1].Person != "Alison" || entries[1].Name != "Beta chore" {
		t.Errorf("expected second entry Alison/Beta chore, got %s/%s", entries[1].Person, entries[1].Name)
	}
	if entries[2].Person != "Kevina" || entries[2].Name != "Zebra chore" {
		t.Errorf("expected third entry Kevina/Zebra chore, got %s/%s", entries[2].Person, entries[2].Name)
	}
}

func TestBuildSchedule_MultipleChoresSameDay(t *testing.T) {
	chores := []Chore{
		{Name: "Dishes", Person: "Alison", CadenceWeeks: 1},
		{Name: "Laundry", Person: "Alison", CadenceWeeks: 1},
	}
	s := buildSchedule(chores)

	entries, ok := s["2026-01-01"]
	if !ok {
		t.Fatal("expected 2026-01-01 in schedule")
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries on 2026-01-01, got %d", len(entries))
	}
}

func TestBuildSchedule_NoOutOfYearDates(t *testing.T) {
	chores := []Chore{{Name: "Test", Person: "Alice", CadenceWeeks: 1}}
	s := buildSchedule(chores)

	for k := range s {
		year := k[:4]
		if year != "2026" {
			t.Errorf("found out-of-year date %s in schedule", k)
		}
	}
}
