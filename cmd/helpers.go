package cmd

import (
	"fmt"
	"time"

	"github.com/hsanson/go-khal/internal/calendar"
	"github.com/hsanson/go-khal/internal/config"
)

func loadStore() (*config.Config, *calendar.Store, calendar.Dataset, error) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, nil, calendar.Dataset{}, err
	}
	store := calendar.NewStore(cfg)
	ds, err := store.Load()
	if err != nil {
		return nil, nil, calendar.Dataset{}, err
	}
	return cfg, store, ds, nil
}

func parseDateArg(s string) (time.Time, error) {
	if s == "" {
		return time.Now(), nil
	}
	layouts := []string{"2006-01-02", time.RFC3339}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid date format %q (expected YYYY-MM-DD)", s)
}
