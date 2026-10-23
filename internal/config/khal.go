package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type khalDocument struct {
	lines           []string
	trailingNewline bool
}

type khalSection struct {
	level  int
	name   string
	parent string
	start  int
	end    int
}

func loadKhalConfig(data []byte) (*Config, error) {
	doc := newKhalDocument(data)
	sections := doc.sections()
	cfg := defaultConfig()
	cfg.document = doc

	calendarSection := false
	for _, section := range sections {
		switch {
		case section.level == 1 && strings.EqualFold(section.name, "calendars"):
			calendarSection = true
		case section.level == 1 && strings.EqualFold(section.name, "locale"):
			if err := applyLocale(cfg, doc.values(section)); err != nil {
				return nil, err
			}
		case section.level == 1 && strings.EqualFold(section.name, "go-khal"):
			if err := applyGoKhalSettings(cfg, doc.values(section)); err != nil {
				return nil, err
			}
		}
	}
	if !calendarSection {
		return nil, errors.New("missing [calendars] section")
	}

	calendarCount := 0
	for _, section := range sections {
		if section.level != 2 || !strings.EqualFold(section.parent, "calendars") {
			continue
		}
		calendarCount++
		sources, err := sourcesFromKhalSection(section.name, doc.values(section), cfg.hiddenCalendarPaths)
		if err != nil {
			return nil, fmt.Errorf("calendar %q: %w", section.name, err)
		}
		cfg.Sources = append(cfg.Sources, sources...)
	}
	if calendarCount == 0 {
		return nil, errors.New("[calendars] must contain at least one [[calendar]] subsection")
	}
	if err := normalize(cfg); err != nil {
		return nil, err
	}
	cfg.original = cloneConfig(cfg)
	return cfg, nil
}

func applyLocale(cfg *Config, values map[string]string) error {
	if raw, ok := values["firstweekday"]; ok {
		weekday, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || weekday < 0 || weekday > 6 {
			return fmt.Errorf("locale firstweekday must be an integer from 0 to 6")
		}
		cfg.WeekStartsOn = []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}[weekday]
	}
	formats := []struct {
		key    string
		target *string
	}{
		{"timeformat", &cfg.TimeFormat},
		{"dateformat", &cfg.DateFormat},
		{"longdateformat", &cfg.LongDateFormat},
		{"datetimeformat", &cfg.DateTimeFormat},
		{"longdatetimeformat", &cfg.LongDateTimeFormat},
	}
	for _, format := range formats {
		raw, ok := values[format.key]
		if !ok {
			continue
		}
		layout, err := strftimeToGo(raw)
		if err != nil {
			return fmt.Errorf("locale %s: %w", format.key, err)
		}
		*format.target = layout
	}
	return nil
}

func applyGoKhalSettings(cfg *Config, values map[string]string) error {
	if value := strings.TrimSpace(values["default_view"]); value != "" {
		cfg.DefaultView = value
	}
	integerSettings := []struct {
		key    string
		target *int
	}{
		{"sidebar_width", &cfg.SidebarWidth},
		{"recurrence_lookback_months", &cfg.RecurrenceLookbackMonths},
		{"recurrence_lookahead_months", &cfg.RecurrenceLookaheadMonths},
	}
	for _, setting := range integerSettings {
		raw, ok := values[setting.key]
		if !ok {
			continue
		}
		value, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return fmt.Errorf("go-khal %s must be an integer", setting.key)
		}
		*setting.target = value
	}
	if value := strings.TrimSpace(values["minimap_start_time"]); value != "" {
		cfg.MinimapStartTime = value
	}
	if value := strings.TrimSpace(values["minimap_end_time"]); value != "" {
		cfg.MinimapEndTime = value
	}
	for _, raw := range parseINIList(values["hidden_calendar_paths"]) {
		path, err := expandConfigPath(raw)
		if err != nil {
			return fmt.Errorf("go-khal hidden_calendar_paths: %w", err)
		}
		cfg.hiddenCalendarPaths = append(cfg.hiddenCalendarPaths, filepath.Clean(path))
	}
	return nil
}

func sourcesFromKhalSection(name string, values map[string]string, hiddenPaths []string) ([]Source, error) {
	rawPath := strings.TrimSpace(values["path"])
	if rawPath == "" {
		return nil, errors.New("path is required")
	}
	expanded, err := expandConfigPath(rawPath)
	if err != nil {
		return nil, err
	}
	typeName := strings.ToLower(strings.TrimSpace(values["type"]))
	if typeName == "" {
		typeName = "calendar"
	}
	readOnly, err := parseINIBool(values["readonly"])
	if err != nil {
		return nil, fmt.Errorf("readonly: %w", err)
	}
	addresses := parseINIList(values["addresses"])
	color := strings.TrimSpace(values["color"])
	if strings.EqualFold(color, "auto") {
		color = ""
	}

	newSource := func(path, sourceType string, discovered bool) Source {
		hidden := false
		cleanPath := filepath.Clean(path)
		for _, hiddenPath := range hiddenPaths {
			if cleanPath == filepath.Clean(hiddenPath) {
				hidden = true
				break
			}
		}
		source := Source{
			Name:        name,
			Path:        cleanPath,
			Type:        sourceType,
			DisplayName: name,
			Color:       color,
			Addresses:   append([]string(nil), addresses...),
			Hidden:      hidden,
			ReadOnly:    readOnly,
			Discovered:  discovered,
		}
		if len(addresses) > 0 {
			source.Email = addresses[0]
		}
		return source
	}

	switch typeName {
	case "calendar":
		if hasGlobMeta(expanded) {
			return nil, errors.New("path globs require type = discover")
		}
		return []Source{newSource(expanded, "calendar", false)}, nil
	case "birthdays":
		if hasGlobMeta(expanded) {
			return nil, errors.New("birthday paths cannot contain globs")
		}
		source := newSource(expanded, "addressbook", false)
		source.ReadOnly = true
		return []Source{source}, nil
	case "discover":
		paths, err := discoverDirectories(expanded)
		if err != nil {
			return nil, err
		}
		out := make([]Source, 0, len(paths))
		for _, path := range paths {
			source := newSource(path, "calendar", true)
			source.Name = filepath.Base(path)
			source.DisplayName = ""
			// khal does not inherit addresses from discover sections.
			source.Email = ""
			source.Addresses = nil
			out = append(out, source)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported type %q", typeName)
	}
}

func expandConfigPath(value string) (string, error) {
	value = os.ExpandEnv(strings.TrimSpace(value))
	if value == "" {
		return "", errors.New("path is empty after expansion")
	}
	if value[0] == '~' {
		slash := strings.IndexAny(value, `/\\`)
		account := value[1:]
		rest := ""
		if slash >= 0 {
			account = value[1:slash]
			rest = value[slash+1:]
		}
		var home string
		if account == "" {
			current, err := os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("resolve home directory: %w", err)
			}
			home = current
		} else {
			entry, err := user.Lookup(account)
			if err != nil {
				return "", fmt.Errorf("resolve user %q: %w", account, err)
			}
			home = entry.HomeDir
		}
		value = filepath.Join(home, rest)
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value), nil
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", value, err)
	}
	return filepath.Clean(absolute), nil
}

func discoverDirectories(pattern string) ([]string, error) {
	var matches []string
	var err error
	if strings.Contains(filepath.ToSlash(pattern), "**") {
		matches, err = recursiveGlob(pattern)
	} else {
		matches, err = filepath.Glob(pattern)
	}
	if err != nil {
		return nil, fmt.Errorf("invalid discover path %q: %w", pattern, err)
	}
	out := matches[:0]
	for _, match := range matches {
		info, statErr := os.Stat(match)
		if statErr == nil && info.IsDir() {
			out = append(out, filepath.Clean(match))
		}
	}
	sort.Strings(out)
	return out, nil
}

func recursiveGlob(pattern string) ([]string, error) {
	cleanPattern := filepath.Clean(pattern)
	root := cleanPattern
	for hasGlobMeta(root) {
		parent := filepath.Dir(root)
		if parent == root {
			break
		}
		root = parent
	}
	if hasGlobMeta(root) {
		root = string(filepath.Separator)
	}
	var matches []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		matched, matchErr := matchDoubleStar(filepath.ToSlash(cleanPattern), filepath.ToSlash(filepath.Clean(path)))
		if matchErr != nil {
			return matchErr
		}
		if matched {
			matches = append(matches, path)
		}
		return nil
	})
	return matches, err
}

func matchDoubleStar(pattern, value string) (bool, error) {
	patternParts := strings.Split(pattern, "/")
	valueParts := strings.Split(value, "/")
	var match func(int, int) (bool, error)
	match = func(patternIndex, valueIndex int) (bool, error) {
		if patternIndex == len(patternParts) {
			return valueIndex == len(valueParts), nil
		}
		if patternParts[patternIndex] == "**" {
			for next := valueIndex; next <= len(valueParts); next++ {
				matched, err := match(patternIndex+1, next)
				if err != nil || matched {
					return matched, err
				}
			}
			return false, nil
		}
		if valueIndex == len(valueParts) {
			return false, nil
		}
		matched, err := pathpkg.Match(patternParts[patternIndex], valueParts[valueIndex])
		if err != nil || !matched {
			return false, err
		}
		return match(patternIndex+1, valueIndex+1)
	}
	return match(0, 0)
}

func hasGlobMeta(value string) bool {
	return strings.ContainsAny(value, "*?[")
}

func strftimeToGo(format string) (string, error) {
	var out strings.Builder
	for index := 0; index < len(format); index++ {
		if format[index] != '%' {
			out.WriteByte(format[index])
			continue
		}
		index++
		if index >= len(format) {
			return "", errors.New("trailing % directive")
		}
		flag := byte(0)
		if strings.ContainsRune("-_0", rune(format[index])) {
			flag = format[index]
			index++
			if index >= len(format) {
				return "", errors.New("incomplete % directive")
			}
		}
		directive := format[index]
		layout, ok := map[byte]string{
			'%': "%", 'a': "Mon", 'A': "Monday", 'b': "Jan", 'B': "January",
			'c': "Mon Jan _2 15:04:05 2006", 'd': "02", 'e': "_2", 'F': "2006-01-02",
			'H': "15", 'I': "03", 'j': "002", 'm': "01", 'M': "04", 'p': "PM",
			'r': "03:04:05 PM", 'R': "15:04", 'S': "05", 'T': "15:04:05",
			'x': "2006-01-02", 'X': "15:04:05", 'y': "06", 'Y': "2006",
			'z': "-0700", 'Z': "MST", 'n': "\n", 't': "\t",
		}[directive]
		if !ok {
			return "", fmt.Errorf("unsupported strftime directive %%%c", directive)
		}
		if flag == '-' {
			switch directive {
			case 'd', 'e':
				layout = "2"
			case 'm':
				layout = "1"
			case 'I':
				layout = "3"
			}
		} else if flag == '_' {
			switch directive {
			case 'd', 'e':
				layout = "_2"
			default:
				return "", fmt.Errorf("unsupported strftime flag _ for %%%c", directive)
			}
		} else if flag != 0 && flag != '0' {
			return "", fmt.Errorf("unsupported strftime flag %c", flag)
		}
		out.WriteString(layout)
	}
	return out.String(), nil
}

func goTimeToStrftime(layout string) (string, error) {
	replacements := []struct{ goLayout, strftime string }{
		{"Monday", "%A"}, {"January", "%B"}, {"2006", "%Y"}, {"MST", "%Z"},
		{"-0700", "%z"}, {"15", "%H"}, {"03", "%I"}, {"04", "%M"},
		{"05", "%S"}, {"PM", "%p"}, {"Mon", "%a"}, {"Jan", "%b"},
		{"002", "%j"}, {"01", "%m"}, {"02", "%d"}, {"06", "%y"},
		{"_2", "%e"},
	}
	remaining := layout
	var out strings.Builder
	for len(remaining) > 0 {
		matched := false
		for _, replacement := range replacements {
			if strings.HasPrefix(remaining, replacement.goLayout) {
				out.WriteString(replacement.strftime)
				remaining = remaining[len(replacement.goLayout):]
				matched = true
				break
			}
		}
		if matched {
			continue
		}
		if remaining[0] == '%' {
			out.WriteString("%%")
		} else {
			out.WriteByte(remaining[0])
		}
		remaining = remaining[1:]
	}
	converted, err := strftimeToGo(out.String())
	if err != nil || converted != layout {
		return "", fmt.Errorf("unsupported Go time layout %q; use a layout composed of standard date/time tokens", layout)
	}
	return out.String(), nil
}

func encodeKhalConfig(cfg *Config) ([]byte, error) {
	if err := normalize(cfg); err != nil {
		return nil, err
	}
	var doc *khalDocument
	if cfg.document == nil {
		var err error
		doc, err = newDocumentFromConfig(cfg)
		if err != nil {
			return nil, err
		}
	} else {
		doc = &khalDocument{
			lines:           append([]string(nil), cfg.document.lines...),
			trailingNewline: cfg.document.trailingNewline,
		}
		if err := updateDocument(doc, cfg); err != nil {
			return nil, err
		}
	}
	result := strings.Join(doc.lines, "\n")
	if doc.trailingNewline || result != "" {
		result += "\n"
	}
	return []byte(result), nil
}

func newDocumentFromConfig(cfg *Config) (*khalDocument, error) {
	sources := make([]Source, 0, len(cfg.Sources))
	for _, source := range cfg.Sources {
		if !source.Discovered {
			sources = append(sources, source)
		}
	}
	if len(sources) == 0 {
		return nil, errors.New("cannot write khal config without at least one calendar; use config add-source or config from-vdirsyncer")
	}
	doc := &khalDocument{lines: []string{"[calendars]"}, trailingNewline: true}
	usedNames := make(map[string]bool, len(sources))
	for _, source := range sources {
		name := source.DisplayName
		if name == "" {
			name = source.Name
		}
		if name == "" {
			name = filepath.Base(source.Path)
		}
		base := name
		for suffix := 1; usedNames[strings.ToLower(name)]; suffix++ {
			name = fmt.Sprintf("%s%d", base, suffix)
		}
		usedNames[strings.ToLower(name)] = true
		doc.lines = append(doc.lines, "[["+quoteSectionName(name)+"]]")
		appendSourceLines(doc, source)
	}

	formats, err := configStrftimeFormats(cfg)
	if err != nil {
		return nil, err
	}
	doc.lines = append(doc.lines, "", "[locale]",
		"firstweekday = "+strconv.Itoa(weekdayNumber(cfg.WeekStart())),
		"timeformat = "+quoteINI(formats["timeformat"]),
		"dateformat = "+quoteINI(formats["dateformat"]),
		"longdateformat = "+quoteINI(formats["longdateformat"]),
		"datetimeformat = "+quoteINI(formats["datetimeformat"]),
		"longdatetimeformat = "+quoteINI(formats["longdatetimeformat"]),
		"", "[go-khal]",
	)
	appendGoKhalLines(doc, cfg)
	return doc, nil
}

func appendSourceLines(doc *khalDocument, source Source) {
	typeName := "calendar"
	if source.Type == "addressbook" {
		typeName = "birthdays"
	}
	doc.lines = append(doc.lines, "path = "+quoteINI(source.Path), "type = "+typeName)
	if source.Color != "" {
		doc.lines = append(doc.lines, "color = "+quoteINI(source.Color))
	}
	if source.ReadOnly || typeName == "birthdays" {
		doc.lines = append(doc.lines, "readonly = true")
	}
	addresses := source.Addresses
	if len(addresses) == 0 && source.Email != "" {
		addresses = []string{source.Email}
	}
	if len(addresses) > 0 {
		doc.lines = append(doc.lines, "addresses = "+formatINIList(addresses))
	}
}

func appendGoKhalLines(doc *khalDocument, cfg *Config) {
	doc.lines = append(doc.lines,
		"default_view = "+quoteINI(cfg.DefaultView),
		"sidebar_width = "+strconv.Itoa(cfg.SidebarWidth),
		"minimap_start_time = "+quoteINI(cfg.MinimapStartTime),
		"minimap_end_time = "+quoteINI(cfg.MinimapEndTime),
		"recurrence_lookback_months = "+strconv.Itoa(cfg.RecurrenceLookbackMonths),
		"recurrence_lookahead_months = "+strconv.Itoa(cfg.RecurrenceLookaheadMonths),
		"hidden_calendar_paths = "+formatINIList(hiddenPaths(cfg.Sources)),
	)
}

func updateDocument(doc *khalDocument, cfg *Config) error {
	original := cfg.original
	if original == nil {
		original = defaultConfig()
	}
	originalByPath := make(map[string]Source, len(original.Sources))
	for _, source := range original.Sources {
		originalByPath[filepath.Clean(source.Path)] = source
	}
	for _, source := range cfg.Sources {
		if source.Discovered {
			continue
		}
		previous, exists := originalByPath[filepath.Clean(source.Path)]
		if !exists {
			if err := doc.addCalendar(source); err != nil {
				return err
			}
			continue
		}
		section, ok := doc.calendarSectionByPath(previous.Path)
		if !ok {
			continue
		}
		if source.DisplayName != "" && source.DisplayName != previous.DisplayName {
			doc.renameSection(section, source.DisplayName)
			section, _ = doc.calendarSectionByPath(previous.Path)
		}
		if source.Path != previous.Path {
			doc.setKey(section, "path", quoteINI(source.Path))
		}
		if source.Type != previous.Type {
			typeName := "calendar"
			if source.Type == "addressbook" {
				typeName = "birthdays"
			}
			doc.setKey(section, "type", typeName)
		}
		if source.Color != previous.Color {
			doc.setKey(section, "color", quoteINI(source.Color))
		}
		if source.ReadOnly != previous.ReadOnly {
			doc.setKey(section, "readonly", strconv.FormatBool(source.ReadOnly))
		}
		if !stringSlicesEqual(sourceAddresses(source), sourceAddresses(previous)) {
			doc.setKey(section, "addresses", formatINIList(sourceAddresses(source)))
		}
	}

	settings := []struct {
		key     string
		current string
		old     string
	}{
		{"default_view", cfg.DefaultView, original.DefaultView},
		{"sidebar_width", strconv.Itoa(cfg.SidebarWidth), strconv.Itoa(original.SidebarWidth)},
		{"minimap_start_time", cfg.MinimapStartTime, original.MinimapStartTime},
		{"minimap_end_time", cfg.MinimapEndTime, original.MinimapEndTime},
		{"recurrence_lookback_months", strconv.Itoa(cfg.RecurrenceLookbackMonths), strconv.Itoa(original.RecurrenceLookbackMonths)},
		{"recurrence_lookahead_months", strconv.Itoa(cfg.RecurrenceLookaheadMonths), strconv.Itoa(original.RecurrenceLookaheadMonths)},
	}
	for _, setting := range settings {
		if setting.current != setting.old {
			section := doc.ensureTopSection("go-khal")
			doc.setKey(section, setting.key, quoteINI(setting.current))
		}
	}
	currentHidden := hiddenPaths(cfg.Sources)
	oldHidden := hiddenPaths(original.Sources)
	if !stringSlicesEqual(currentHidden, oldHidden) {
		section := doc.ensureTopSection("go-khal")
		doc.setKey(section, "hidden_calendar_paths", formatINIList(currentHidden))
	}

	if cfg.WeekStartsOn != original.WeekStartsOn || cfg.TimeFormat != original.TimeFormat || cfg.DateFormat != original.DateFormat ||
		cfg.LongDateFormat != original.LongDateFormat || cfg.DateTimeFormat != original.DateTimeFormat || cfg.LongDateTimeFormat != original.LongDateTimeFormat {
		formats, err := configStrftimeFormats(cfg)
		if err != nil {
			return err
		}
		section := doc.ensureTopSection("locale")
		doc.setKey(section, "firstweekday", strconv.Itoa(weekdayNumber(cfg.WeekStart())))
		for key, value := range formats {
			doc.setKey(section, key, quoteINI(value))
		}
	}
	return nil
}

func configStrftimeFormats(cfg *Config) (map[string]string, error) {
	layouts := map[string]string{
		"timeformat":         cfg.TimeFormat,
		"dateformat":         cfg.DateFormat,
		"longdateformat":     cfg.LongDateFormat,
		"datetimeformat":     cfg.DateTimeFormat,
		"longdatetimeformat": cfg.LongDateTimeFormat,
	}
	formats := make(map[string]string, len(layouts))
	for key, layout := range layouts {
		format, err := goTimeToStrftime(layout)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		formats[key] = format
	}
	return formats, nil
}

func sourceAddresses(source Source) []string {
	if len(source.Addresses) > 0 {
		return source.Addresses
	}
	if source.Email != "" {
		return []string{source.Email}
	}
	return nil
}

func hiddenPaths(sources []Source) []string {
	paths := make([]string, 0)
	for _, source := range sources {
		if source.Hidden {
			paths = append(paths, filepath.Clean(source.Path))
		}
	}
	sort.Strings(paths)
	return paths
}

func weekdayNumber(day time.Weekday) int {
	if day == time.Sunday {
		return 6
	}
	return int(day) - 1
}

func cloneConfig(cfg *Config) *Config {
	clone := *cfg
	clone.Sources = append([]Source(nil), cfg.Sources...)
	for index := range clone.Sources {
		clone.Sources[index].Addresses = append([]string(nil), cfg.Sources[index].Addresses...)
	}
	clone.hiddenCalendarPaths = append([]string(nil), cfg.hiddenCalendarPaths...)
	clone.document = nil
	clone.original = nil
	return &clone
}

func newKhalDocument(data []byte) *khalDocument {
	text := string(data)
	trailing := strings.HasSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\n")
	text = strings.TrimSuffix(text, "\r")
	lines := []string{}
	if text != "" {
		lines = strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	}
	return &khalDocument{lines: lines, trailingNewline: trailing}
}

func (doc *khalDocument) sections() []khalSection {
	sections := make([]khalSection, 0)
	stack := make([]int, 0)
	tripleQuote := ""
	for lineNumber, line := range doc.lines {
		if tripleQuote != "" {
			if strings.Count(line, tripleQuote)%2 == 1 {
				tripleQuote = ""
			}
			continue
		}
		if _, raw, ok := parseAssignment(line); ok {
			trimmed := strings.TrimSpace(raw)
			if strings.HasPrefix(trimmed, `"""`) && strings.Count(trimmed, `"""`)%2 == 1 {
				tripleQuote = `"""`
			} else if strings.HasPrefix(trimmed, "'''") && strings.Count(trimmed, "'''")%2 == 1 {
				tripleQuote = "'''"
			}
			continue
		}
		level, name, ok := parseSectionHeader(line)
		if !ok {
			continue
		}
		for len(stack) > 0 && sections[stack[len(stack)-1]].level >= level {
			sections[stack[len(stack)-1]].end = lineNumber
			stack = stack[:len(stack)-1]
		}
		parent := ""
		if len(stack) > 0 {
			parent = sections[stack[len(stack)-1]].name
		}
		sections = append(sections, khalSection{level: level, name: name, parent: parent, start: lineNumber, end: len(doc.lines)})
		stack = append(stack, len(sections)-1)
	}
	for _, index := range stack {
		sections[index].end = len(doc.lines)
	}
	return sections
}

func (doc *khalDocument) values(section khalSection) map[string]string {
	values := make(map[string]string)
	for lineNumber := section.start + 1; lineNumber < section.end; lineNumber++ {
		if _, _, ok := parseSectionHeader(doc.lines[lineNumber]); ok {
			break
		}
		key, raw, ok := parseAssignment(doc.lines[lineNumber])
		if !ok {
			continue
		}
		values[strings.ToLower(key)] = parseINIValue(raw)
	}
	return values
}

func (doc *khalDocument) calendarSectionByPath(path string) (khalSection, bool) {
	cleanPath := filepath.Clean(path)
	for _, section := range doc.sections() {
		if section.level != 2 || !strings.EqualFold(section.parent, "calendars") {
			continue
		}
		rawPath := doc.values(section)["path"]
		expanded, err := expandConfigPath(rawPath)
		if err == nil && filepath.Clean(expanded) == cleanPath {
			return section, true
		}
	}
	return khalSection{}, false
}

func (doc *khalDocument) ensureTopSection(name string) khalSection {
	for _, section := range doc.sections() {
		if section.level == 1 && strings.EqualFold(section.name, name) {
			return section
		}
	}
	if len(doc.lines) > 0 && strings.TrimSpace(doc.lines[len(doc.lines)-1]) != "" {
		doc.lines = append(doc.lines, "")
	}
	doc.lines = append(doc.lines, "["+quoteSectionName(name)+"]")
	for _, section := range doc.sections() {
		if section.level == 1 && strings.EqualFold(section.name, name) {
			return section
		}
	}
	panic("added section not found")
}

func (doc *khalDocument) addCalendar(source Source) error {
	calendars := doc.ensureTopSection("calendars")
	name := source.DisplayName
	if name == "" {
		name = source.Name
	}
	if name == "" {
		name = filepath.Base(source.Path)
	}
	used := make(map[string]bool)
	for _, section := range doc.sections() {
		if section.level == 2 && strings.EqualFold(section.parent, "calendars") {
			used[strings.ToLower(section.name)] = true
		}
	}
	base := name
	for suffix := 1; used[strings.ToLower(name)]; suffix++ {
		name = fmt.Sprintf("%s%d", base, suffix)
	}
	lines := []string{"[[" + quoteSectionName(name) + "]]"}
	temporary := &khalDocument{lines: lines}
	appendSourceLines(temporary, source)
	insertAt := calendars.end
	if insertAt > calendars.start+1 && strings.TrimSpace(doc.lines[insertAt-1]) != "" {
		temporary.lines = append([]string{""}, temporary.lines...)
	}
	doc.insert(insertAt, temporary.lines)
	return nil
}

func (doc *khalDocument) renameSection(section khalSection, name string) {
	line := doc.lines[section.start]
	comment := inlineComment(line)
	doc.lines[section.start] = "[[" + quoteSectionName(name) + "]]" + comment
}

func (doc *khalDocument) setKey(section khalSection, key, value string) {
	for lineNumber := section.start + 1; lineNumber < section.end; lineNumber++ {
		if _, _, ok := parseSectionHeader(doc.lines[lineNumber]); ok {
			break
		}
		lineKey, _, ok := parseAssignment(doc.lines[lineNumber])
		if !ok || !strings.EqualFold(lineKey, key) {
			continue
		}
		line := doc.lines[lineNumber]
		equals := assignmentEquals(line)
		if equals < 0 {
			break
		}
		after := line[equals+1:]
		leading := len(after) - len(strings.TrimLeft(after, " \t"))
		commentAt := inlineCommentIndex(after)
		suffix := ""
		if commentAt >= 0 {
			suffix = strings.TrimRight(after[:commentAt], " \t")
			suffix = after[len(suffix):]
		}
		doc.lines[lineNumber] = line[:equals+1] + after[:leading] + value + suffix
		return
	}
	indent := ""
	if section.level > 1 {
		indent = ""
	}
	doc.insert(section.end, []string{indent + key + " = " + value})
}

func (doc *khalDocument) insert(index int, lines []string) {
	doc.lines = append(doc.lines, make([]string, len(lines))...)
	copy(doc.lines[index+len(lines):], doc.lines[index:len(doc.lines)-len(lines)])
	copy(doc.lines[index:], lines)
}

func parseSectionHeader(line string) (int, string, bool) {
	trimmed := strings.TrimSpace(stripInlineComment(line))
	if !strings.HasPrefix(trimmed, "[") {
		return 0, "", false
	}
	level := 0
	for level < len(trimmed) && trimmed[level] == '[' {
		level++
	}
	closing := 0
	for closing < len(trimmed) && trimmed[len(trimmed)-1-closing] == ']' {
		closing++
	}
	if level == 0 || closing != level || len(trimmed) <= level+closing {
		return 0, "", false
	}
	name := unquoteINI(strings.TrimSpace(trimmed[level : len(trimmed)-closing]))
	if name == "" {
		return 0, "", false
	}
	return level, name, true
}

func parseAssignment(line string) (string, string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", "", false
	}
	equals := assignmentEquals(line)
	if equals < 0 {
		return "", "", false
	}
	key := unquoteINI(strings.TrimSpace(line[:equals]))
	if key == "" {
		return "", "", false
	}
	return key, line[equals+1:], true
}

func assignmentEquals(line string) int {
	quote := byte(0)
	for index := 0; index < len(line); index++ {
		switch line[index] {
		case '\'', '"':
			if quote == 0 {
				quote = line[index]
			} else if quote == line[index] && (index == 0 || line[index-1] != '\\') {
				quote = 0
			}
		case '=':
			if quote == 0 {
				return index
			}
		case '#':
			if quote == 0 {
				return -1
			}
		}
	}
	return -1
}

func parseINIValue(raw string) string {
	return unquoteINI(strings.TrimSpace(stripInlineComment(raw)))
}

func stripInlineComment(value string) string {
	if index := inlineCommentIndex(value); index >= 0 {
		return value[:index]
	}
	return value
}

func inlineComment(value string) string {
	if index := inlineCommentIndex(value); index >= 0 {
		return value[index:]
	}
	return ""
}

func inlineCommentIndex(value string) int {
	quote := byte(0)
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '\'', '"':
			if quote == 0 {
				quote = value[index]
			} else if quote == value[index] && (index == 0 || value[index-1] != '\\') {
				quote = 0
			}
		case '#':
			if quote == 0 {
				return index
			}
		}
	}
	return -1
}

func unquoteINI(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 2 || (value[0] != value[len(value)-1]) || (value[0] != '\'' && value[0] != '"') {
		return value
	}
	if value[0] == '"' {
		if unquoted, err := strconv.Unquote(value); err == nil {
			return unquoted
		}
	}
	return strings.ReplaceAll(value[1:len(value)-1], `\'`, `'`)
}

func parseINIList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var out []string
	start := 0
	quote := byte(0)
	for index := 0; index <= len(value); index++ {
		if index == len(value) || (value[index] == ',' && quote == 0) {
			item := unquoteINI(strings.TrimSpace(value[start:index]))
			if item != "" {
				out = append(out, item)
			}
			start = index + 1
			continue
		}
		if value[index] == '\'' || value[index] == '"' {
			if quote == 0 {
				quote = value[index]
			} else if quote == value[index] && (index == 0 || value[index-1] != '\\') {
				quote = 0
			}
		}
	}
	return out
}

func parseINIBool(value string) (bool, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return false, nil
	}
	switch value {
	case "true", "on", "yes", "1":
		return true, nil
	case "false", "off", "no", "0":
		return false, nil
	default:
		return false, fmt.Errorf("invalid boolean %q", value)
	}
}

func quoteINI(value string) string {
	if value == "" {
		return `''`
	}
	if strings.TrimSpace(value) != value || strings.ContainsAny(value, "#,\n\r\"") {
		return strconv.Quote(value)
	}
	return value
}

func quoteSectionName(value string) string {
	if strings.ContainsAny(value, "[]#\n\r\"") || strings.TrimSpace(value) != value {
		return strconv.Quote(value)
	}
	return value
}

func formatINIList(values []string) string {
	encoded := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			encoded = append(encoded, quoteINI(strings.TrimSpace(value)))
		}
	}
	return strings.Join(encoded, ", ")
}

func stringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
