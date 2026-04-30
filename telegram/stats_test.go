package telegram

import (
	"testing"
	"time"
)

func TestParseStatsMessage(t *testing.T) {
	text := `2021:
20 gennaio: 140 circa

2023:
27 aprile: 1900
31 luglio: 2000 Dario Moccia incident
11 agosto: 2100

2026:
30 aprile 5300 Mario's`

	rows, err := parseStatsMessage(text)
	if err != nil {
		t.Fatalf("Failed to parse message: %v", err)
	}

	if len(rows) != 5 {
		t.Errorf("Expected 5 rows, got %d", len(rows))
	}

	expected := []struct {
		date  time.Time
		subs  float64
		note  string
	}{
		{time.Date(2021, time.January, 20, 0, 0, 0, 0, time.UTC), 140, "circa"},
		{time.Date(2023, time.April, 27, 0, 0, 0, 0, time.UTC), 1900, ""},
		{time.Date(2023, time.July, 31, 0, 0, 0, 0, time.UTC), 2000, "Dario Moccia incident"},
		{time.Date(2023, time.August, 11, 0, 0, 0, 0, time.UTC), 2100, ""},
		{time.Date(2026, time.April, 30, 0, 0, 0, 0, time.UTC), 5300, "Mario's"},
	}

	for i, exp := range expected {
		if !rows[i].Date.Equal(exp.date) {
			t.Errorf("Row %d: expected date %v, got %v", i, exp.date, rows[i].Date)
		}
		if rows[i].Subscribers != exp.subs {
			t.Errorf("Row %d: expected %f subscribers, got %f", i, exp.subs, rows[i].Subscribers)
		}
		if rows[i].Note != exp.note {
			t.Errorf("Row %d: expected note %q, got %q", i, exp.note, rows[i].Note)
		}
	}
}

func TestPlots(t *testing.T) {
	rows := []StatsRow{
		{time.Date(2021, time.January, 20, 0, 0, 0, 0, time.UTC), 140, "circa"},
		{time.Date(2023, time.April, 27, 0, 0, 0, 0, time.UTC), 1900, ""},
		{time.Date(2023, time.July, 31, 0, 0, 0, 0, time.UTC), 2000, "Dario Moccia incident"},
		{time.Date(2023, time.August, 11, 0, 0, 0, 0, time.UTC), 2100, ""},
		{time.Date(2026, time.April, 30, 0, 0, 0, 0, time.UTC), 5300, "Mario's"},
	}

	_, err := createLinearPlot(rows)
	if err != nil {
		t.Errorf("Failed to create linear plot: %v", err)
	}

	_, err = createPredictionPlot(rows, 4)
	if err != nil {
		t.Errorf("Failed to create prediction plot: %v", err)
	}
}
