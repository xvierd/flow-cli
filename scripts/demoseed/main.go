// Command demoseed fills a throwaway flow database with a year of realistic,
// deterministic focus history. It powers docs/demo.tape and smoke tests:
//
//	go run ./scripts/demoseed --db /tmp/demo.db
//
// It refuses to write to an existing file so it can never touch a real DB.
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/xvierd/flow-cli/internal/adapters/storage"
	"github.com/xvierd/flow-cli/internal/domain"
)

func main() {
	dbPath := flag.String("db", "", "path of the NEW database file to create (required)")
	days := flag.Int("days", 370, "days of history to generate, ending today")
	seed := flag.Int64("seed", 42, "random seed")
	flag.Parse()
	if err := run(*dbPath, *days, *seed); err != nil {
		fmt.Fprintln(os.Stderr, "demoseed:", err)
		os.Exit(1)
	}
}

func run(dbPath string, days int, seed int64) error {
	if dbPath == "" {
		return fmt.Errorf("--db is required")
	}
	if _, err := os.Stat(dbPath); err == nil {
		return fmt.Errorf("%s already exists; demoseed only creates new databases", dbPath)
	}
	store, err := storage.New(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	ctx := context.Background()
	rng := rand.New(rand.NewSource(seed)) // #nosec G404 -- deterministic fake demo data, not security sensitive

	specs := []struct {
		title string
		tags  []string
	}{
		{"Write API docs", []string{"docs", "backend"}},
		{"Refactor storage layer", []string{"backend"}},
		{"Ship wrapped card", []string{"frontend", "share"}},
		{"Review pull requests", []string{"review"}},
		{"Study Go generics", []string{"learning"}},
	}
	tasks := make([]*domain.Task, 0, len(specs))
	for _, s := range specs {
		t, err := domain.NewTask(s.title)
		if err != nil {
			return err
		}
		t.Tags = s.tags
		if err := store.Tasks().Save(ctx, t); err != nil {
			return err
		}
		tasks = append(tasks, t)
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	n := 0
	for i := days; i >= 1; i-- { // history up to yesterday; today stays empty
		day := today.AddDate(0, 0, -i)
		weekend := day.Weekday() == time.Saturday || day.Weekday() == time.Sunday
		skip := 0.18
		if weekend {
			skip = 0.7
		}
		if i <= 9 {
			skip = 0 // keep a live streak for the demo
		}
		if rng.Float64() < skip {
			continue
		}
		sessions := 1 + rng.Intn(6)
		hour := 8 + rng.Intn(3)
		minute := 0
		for k := 0; k < sessions; k++ {
			ti := rng.Intn(len(tasks))
			dur := []time.Duration{25, 25, 25, 50, 90}[rng.Intn(5)] * time.Minute
			start := day.Add(time.Duration(hour)*time.Hour + time.Duration(minute)*time.Minute)
			end := start.Add(dur)
			id := tasks[ti].ID
			s := &domain.PomodoroSession{
				ID:          uuid.NewString(),
				TaskID:      &id,
				Type:        domain.SessionTypeWork,
				Status:      domain.SessionStatusCompleted,
				Duration:    dur,
				StartedAt:   start,
				CompletedAt: &end,
				Methodology: domain.MethodologyPomodoro,
				Tags:        specs[ti].tags,
			}
			if err := store.Sessions().Save(ctx, s); err != nil {
				return err
			}
			n++
			total := int(dur.Minutes()) + 5 + rng.Intn(20)
			hour += (minute + total) / 60
			minute = (minute + total) % 60
			if k == 2 {
				hour += 1 + rng.Intn(2) // lunch
			}
			if hour >= 23 {
				break
			}
		}
	}
	fmt.Printf("seeded %d sessions over %d days into %s\n", n, days, dbPath)
	return nil
}
