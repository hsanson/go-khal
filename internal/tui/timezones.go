package tui

import (
	"fmt"
	"os"
	"strings"
	"time"
)

var commonIANATimezones = []string{
	"UTC",
	"Africa/Abidjan",
	"Africa/Accra",
	"Africa/Addis_Ababa",
	"Africa/Algiers",
	"Africa/Cairo",
	"Africa/Casablanca",
	"Africa/Dar_es_Salaam",
	"Africa/Johannesburg",
	"Africa/Lagos",
	"Africa/Nairobi",
	"Africa/Tunis",
	"America/Anchorage",
	"America/Argentina/Buenos_Aires",
	"America/Bogota",
	"America/Caracas",
	"America/Chicago",
	"America/Denver",
	"America/Edmonton",
	"America/Guatemala",
	"America/Halifax",
	"America/Havana",
	"America/Lima",
	"America/Los_Angeles",
	"America/Manaus",
	"America/Mexico_City",
	"America/Monterrey",
	"America/Montevideo",
	"America/New_York",
	"America/Panama",
	"America/Phoenix",
	"America/Regina",
	"America/Santiago",
	"America/Sao_Paulo",
	"America/St_Johns",
	"America/Toronto",
	"America/Vancouver",
	"America/Winnipeg",
	"Asia/Almaty",
	"Asia/Amman",
	"Asia/Baghdad",
	"Asia/Baku",
	"Asia/Bangkok",
	"Asia/Beirut",
	"Asia/Colombo",
	"Asia/Dhaka",
	"Asia/Dubai",
	"Asia/Ho_Chi_Minh",
	"Asia/Hong_Kong",
	"Asia/Irkutsk",
	"Asia/Istanbul",
	"Asia/Jakarta",
	"Asia/Jerusalem",
	"Asia/Kabul",
	"Asia/Karachi",
	"Asia/Kathmandu",
	"Asia/Kolkata",
	"Asia/Krasnoyarsk",
	"Asia/Kuala_Lumpur",
	"Asia/Kuwait",
	"Asia/Magadan",
	"Asia/Manila",
	"Asia/Muscat",
	"Asia/Novosibirsk",
	"Asia/Riyadh",
	"Asia/Seoul",
	"Asia/Shanghai",
	"Asia/Singapore",
	"Asia/Taipei",
	"Asia/Tashkent",
	"Asia/Tehran",
	"Asia/Tokyo",
	"Asia/Vladivostok",
	"Asia/Yakutsk",
	"Asia/Yekaterinburg",
	"Atlantic/Azores",
	"Atlantic/Cape_Verde",
	"Atlantic/Reykjavik",
	"Australia/Adelaide",
	"Australia/Brisbane",
	"Australia/Darwin",
	"Australia/Hobart",
	"Australia/Melbourne",
	"Australia/Perth",
	"Australia/Sydney",
	"Europe/Amsterdam",
	"Europe/Athens",
	"Europe/Belgrade",
	"Europe/Berlin",
	"Europe/Brussels",
	"Europe/Bucharest",
	"Europe/Budapest",
	"Europe/Copenhagen",
	"Europe/Dublin",
	"Europe/Helsinki",
	"Europe/Kyiv",
	"Europe/Lisbon",
	"Europe/London",
	"Europe/Madrid",
	"Europe/Minsk",
	"Europe/Moscow",
	"Europe/Oslo",
	"Europe/Paris",
	"Europe/Prague",
	"Europe/Riga",
	"Europe/Rome",
	"Europe/Samara",
	"Europe/Sofia",
	"Europe/Stockholm",
	"Europe/Tallinn",
	"Europe/Vienna",
	"Europe/Vilnius",
	"Europe/Warsaw",
	"Europe/Zurich",
	"Indian/Maldives",
	"Indian/Mauritius",
	"Pacific/Auckland",
	"Pacific/Chatham",
	"Pacific/Fiji",
	"Pacific/Guam",
	"Pacific/Honolulu",
	"Pacific/Midway",
	"Pacific/Noumea",
	"Pacific/Pago_Pago",
	"Pacific/Tongatapu",
}

func timezoneOptions(current, local string, eventTime time.Time) []searchOption {
	names := make([]string, 0, len(commonIANATimezones)+2)
	names = append(names, "UTC", local, current)
	names = append(names, commonIANATimezones...)
	seen := make(map[string]bool, len(names))
	options := make([]searchOption, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" || seen[name] {
			continue
		}
		if _, err := time.LoadLocation(name); err != nil {
			continue
		}
		seen[name] = true
		options = append(options, searchOption{
			label: timezoneLabel(name, eventTime),
			value: name,
		})
	}
	return options
}

func timezoneLabel(name string, eventTime time.Time) string {
	loc, err := time.LoadLocation(name)
	if err != nil {
		return name
	}
	if eventTime.IsZero() {
		eventTime = time.Now()
	}
	probe := time.Date(eventTime.Year(), eventTime.Month(), eventTime.Day(), eventTime.Hour(), eventTime.Minute(), 0, 0, loc)
	_, offset := probe.Zone()
	return fmt.Sprintf("%s  (%s)", name, formatTimezoneOffset(offset))
}

func formatTimezoneOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	hours := seconds / 3600
	minutes := seconds % 3600 / 60
	return fmt.Sprintf("UTC%s%02d:%02d", sign, hours, minutes)
}

func localIANATimezone() string {
	if name := strings.TrimSpace(os.Getenv("TZ")); name != "" && name != "Local" {
		if _, err := time.LoadLocation(name); err == nil {
			return name
		}
	}
	if target, err := os.Readlink("/etc/localtime"); err == nil {
		if index := strings.Index(target, "zoneinfo/"); index >= 0 {
			name := target[index+len("zoneinfo/"):]
			if _, err := time.LoadLocation(name); err == nil {
				return name
			}
		}
	}
	if name := time.Local.String(); name != "" && name != "Local" {
		if _, err := time.LoadLocation(name); err == nil {
			return name
		}
	}
	return "UTC"
}
