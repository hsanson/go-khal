package cmd

import "time"

func parseNow() time.Time {
	return time.Now()
}

func parseOptionalDateTime(s string) (*time.Time, error) {
	if s == "" {
		return nil, nil
	}
	layouts := []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return &t, nil
		}
	}
	return nil, &time.ParseError{Layout: "RFC3339|2006-01-02 15:04|2006-01-02", Value: s}
}
