package cmd

import (
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
