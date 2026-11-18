package edition

import (
	_ "embed"
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"
	"golang.org/x/text/unicode/bidi"

	"techpedia/internal/document"
)

//go:embed data/language-subtag-registry.txt
var languageSubtagRegistry string

type LanguageCatalogEntry struct {
	Tag           string `json:"tag"`
	Name          string `json:"name"`
	NativeName    string `json:"native_name"`
	WritingSystem string `json:"writing_system"`
	ScriptName    string `json:"script_name"`
}

type LanguageProfile struct {
	Tag           string
	Name          string
	NativeName    string
	WritingSystem string
	ScriptName    string
	Direction     string
}

type CountryCatalogEntry struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

type catalogRecord struct {
	tag            string
	description    string
	suppressScript string
	searchText     string
}

type parsedRegistry struct {
	languages []catalogRecord
	scripts   map[string]string
	countries []CountryCatalogEntry
}

var registry = parseLanguageRegistry(languageSubtagRegistry)
var languageCatalog = registry.languages
var languageCatalogByTag = indexLanguageCatalog(languageCatalog)
var countryCatalog = registry.countries
var countryCatalogByCode = indexCountryCatalog(countryCatalog)

func CountryCatalog() []CountryCatalogEntry {
	return append([]CountryCatalogEntry(nil), countryCatalog...)
}

func ValidCountryCode(code string) bool {
	_, found := countryCatalogByCode[strings.ToUpper(strings.TrimSpace(code))]
	return found
}

func SearchLanguageCatalog(query string, excluded map[string]struct{}, limit int) []LanguageCatalogEntry {
	if limit < 1 {
		limit = 30
	}
	if limit > 50 {
		limit = 50
	}
	query = strings.ToLower(strings.TrimSpace(query))
	matches := make([]catalogRecord, 0, limit)
	for _, value := range languageCatalog {
		if _, found := excluded[strings.ToLower(value.tag)]; found {
			continue
		}
		if query != "" && !strings.Contains(value.searchText, query) {
			continue
		}
		matches = append(matches, value)
	}
	sort.SliceStable(matches, func(i, j int) bool {
		iPrefix := query != "" && (strings.HasPrefix(matches[i].tag, query) || strings.HasPrefix(strings.ToLower(matches[i].description), query))
		jPrefix := query != "" && (strings.HasPrefix(matches[j].tag, query) || strings.HasPrefix(strings.ToLower(matches[j].description), query))
		if iPrefix != jPrefix {
			return iPrefix
		}
		return matches[i].description < matches[j].description
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	result := make([]LanguageCatalogEntry, len(matches))
	for i, value := range matches {
		profile, ok := LanguageProfileFor(value.tag)
		if !ok {
			continue
		}
		result[i] = LanguageCatalogEntry{
			Tag:           profile.Tag,
			Name:          profile.Name,
			NativeName:    profile.NativeName,
			WritingSystem: profile.WritingSystem,
			ScriptName:    profile.ScriptName,
		}
	}
	return result
}

func DescribeLanguage(tag string) (name, nativeName, direction string, ok bool) {
	profile, ok := LanguageProfileFor(tag)
	if !ok {
		return "", "", "", false
	}
	return profile.Name, profile.NativeName, profile.Direction, true
}

func LanguageProfileFor(tag string) (LanguageProfile, bool) {
	canonical, ok := document.CanonicalLocale(tag)
	if !ok {
		return LanguageProfile{}, false
	}
	record, found := languageCatalogByTag[canonical]
	if !found {
		return LanguageProfile{}, false
	}
	parsed, err := language.Parse(canonical)
	if err != nil {
		return LanguageProfile{}, false
	}
	name := record.description
	nativeName := display.Self.Name(parsed)
	if nativeName == "" {
		nativeName = name
	}
	writingSystem := record.suppressScript
	if script, confidence := parsed.Script(); writingSystem == "" && confidence != language.No && script.String() != "Zzzz" {
		writingSystem = script.String()
	}
	if writingSystem == "" {
		writingSystem = "Zzzz"
	}
	direction := scriptDirection(writingSystem)
	if direction == "" {
		direction = textDirection(nativeName)
	}
	return LanguageProfile{
		Tag:           canonical,
		Name:          name,
		NativeName:    nativeName,
		WritingSystem: writingSystem,
		ScriptName:    scriptName(writingSystem),
		Direction:     direction,
	}, true
}

func scriptDirection(code string) string {
	for _, ranges := range writingSystemRanges(code) {
		for _, value := range ranges.R16 {
			if direction := rangeDirection(rune(value.Lo), rune(value.Hi), rune(value.Stride)); direction != "" {
				return direction
			}
		}
		for _, value := range ranges.R32 {
			if direction := rangeDirection(rune(value.Lo), rune(value.Hi), rune(value.Stride)); direction != "" {
				return direction
			}
		}
	}
	return ""
}

func WritingSystemRuneCount(tag string, value string) int {
	profile, ok := LanguageProfileFor(tag)
	if !ok {
		return 0
	}
	ranges := writingSystemRanges(profile.WritingSystem)
	matched := 0
	for _, current := range value {
		for _, script := range ranges {
			if unicode.Is(script, current) {
				matched++
				break
			}
		}
	}
	return matched
}

func CanDetectWritingSystem(tag string) bool {
	profile, ok := LanguageProfileFor(tag)
	return ok && len(writingSystemRanges(profile.WritingSystem)) > 0
}

func writingSystemRanges(code string) []*unicode.RangeTable {
	switch code {
	case "Hans", "Hant", "Hani":
		return []*unicode.RangeTable{unicode.Han}
	case "Jpan":
		return []*unicode.RangeTable{unicode.Han, unicode.Hiragana, unicode.Katakana}
	case "Kore":
		return []*unicode.RangeTable{unicode.Hangul, unicode.Han}
	case "Hanb":
		return []*unicode.RangeTable{unicode.Han, unicode.Bopomofo}
	}
	name := scriptName(code)
	if before, _, found := strings.Cut(name, " ("); found {
		name = before
	}
	name = strings.NewReplacer(" ", "_", "-", "_", "’", "", "'", "").Replace(name)
	if ranges := unicode.Scripts[name]; ranges != nil {
		return []*unicode.RangeTable{ranges}
	}
	return nil
}

func scriptName(code string) string {
	if value := registry.scripts[code]; value != "" {
		return value
	}
	if code == "Zzzz" {
		return "Undetermined"
	}
	return code
}

func rangeDirection(low, high, stride rune) string {
	for value := low; value <= high; value += stride {
		property, _ := bidi.LookupRune(value)
		switch property.Class() {
		case bidi.R, bidi.AL:
			return "rtl"
		case bidi.L:
			return "ltr"
		}
	}
	return ""
}

func textDirection(value string) string {
	for len(value) > 0 {
		property, size := bidi.LookupString(value)
		if size == 0 {
			break
		}
		switch property.Class() {
		case bidi.R, bidi.AL:
			return "rtl"
		case bidi.L:
			return "ltr"
		}
		value = value[size:]
	}
	return "ltr"
}

func indexLanguageCatalog(values []catalogRecord) map[string]catalogRecord {
	result := make(map[string]catalogRecord, len(values))
	for _, value := range values {
		result[value.tag] = value
	}
	return result
}

func parseLanguageRegistry(data string) parsedRegistry {
	result := parsedRegistry{
		languages: make([]catalogRecord, 0, 8000),
		scripts:   make(map[string]string, 200),
		countries: make([]CountryCatalogEntry, 0, 260),
	}
	for _, block := range strings.Split(data, "\n%%") {
		fields := make(map[string][]string)
		var lastKey string
		for _, line := range strings.Split(strings.TrimSpace(block), "\n") {
			if strings.HasPrefix(line, " ") && lastKey != "" {
				last := len(fields[lastKey]) - 1
				fields[lastKey][last] += " " + strings.TrimSpace(line)
				continue
			}
			key, value, found := strings.Cut(line, ":")
			if !found {
				continue
			}
			lastKey = key
			fields[key] = append(fields[key], strings.TrimSpace(value))
		}
		recordType := first(fields["Type"])
		tag := first(fields["Subtag"])
		descriptions := fields["Description"]
		if tag == "" || len(descriptions) == 0 || first(fields["Deprecated"]) != "" {
			continue
		}
		if recordType == "script" {
			result.scripts[tag] = descriptions[0]
			continue
		}
		if recordType == "region" && len(tag) == 2 && !strings.EqualFold(descriptions[0], "Private use") {
			result.countries = append(result.countries, CountryCatalogEntry{Code: strings.ToUpper(tag), Name: descriptions[0]})
			continue
		}
		if recordType != "language" {
			continue
		}
		if _, ok := document.CanonicalLocale(tag); !ok {
			continue
		}
		result.languages = append(result.languages, catalogRecord{
			tag:            tag,
			description:    descriptions[0],
			suppressScript: first(fields["Suppress-Script"]),
			searchText:     strings.ToLower(tag + " " + strings.Join(descriptions, " ")),
		})
	}
	sort.Slice(result.countries, func(i, j int) bool { return result.countries[i].Name < result.countries[j].Name })
	return result
}

func indexCountryCatalog(values []CountryCatalogEntry) map[string]CountryCatalogEntry {
	result := make(map[string]CountryCatalogEntry, len(values))
	for _, value := range values {
		result[value.Code] = value
	}
	return result
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
