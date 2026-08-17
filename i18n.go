package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
)

//go:embed locales/catalogs.json
var localizationFiles embed.FS

type LocaleOption struct {
	Code  string `json:"code"`
	Label string `json:"label"`
}

type LocalizationCatalog struct {
	DefaultLocale string                       `json:"defaultLocale"`
	Locales       []LocaleOption               `json:"locales"`
	Messages      map[string]map[string]string `json:"messages"`
}

type LocalizationPayload struct {
	Locale        string            `json:"locale"`
	DefaultLocale string            `json:"defaultLocale"`
	Locales       []LocaleOption    `json:"locales"`
	Messages      map[string]string `json:"messages"`
}

var localizationData = mustLoadLocalization()
var defaultLocale = localizationData.DefaultLocale

func mustLoadLocalization() LocalizationCatalog {
	data, err := localizationFiles.ReadFile("locales/catalogs.json")
	if err != nil {
		panic(fmt.Errorf("read embedded localization catalog: %w", err))
	}
	var catalog LocalizationCatalog
	if err := json.Unmarshal(data, &catalog); err != nil {
		panic(fmt.Errorf("decode embedded localization catalog: %w", err))
	}
	if err := validateLocalization(catalog); err != nil {
		panic(err)
	}
	return catalog
}

func validateLocalization(catalog LocalizationCatalog) error {
	baseline := catalog.Messages[catalog.DefaultLocale]
	if catalog.DefaultLocale == "" || len(baseline) == 0 {
		return fmt.Errorf("localization default locale is missing")
	}
	listed := make(map[string]bool, len(catalog.Locales))
	for _, option := range catalog.Locales {
		if option.Code == "" || option.Label == "" || listed[option.Code] {
			return fmt.Errorf("invalid localization locale entry %q", option.Code)
		}
		listed[option.Code] = true
	}
	for code, messages := range catalog.Messages {
		if !listed[code] {
			return fmt.Errorf("localization catalog %q is not listed", code)
		}
		if len(messages) != len(baseline) {
			return fmt.Errorf("localization catalog %q has %d keys; expected %d", code, len(messages), len(baseline))
		}
		for key := range baseline {
			if messages[key] == "" {
				return fmt.Errorf("localization catalog %q is missing %q", code, key)
			}
		}
	}
	for code := range listed {
		if len(catalog.Messages[code]) == 0 {
			return fmt.Errorf("listed locale %q has no catalog", code)
		}
	}
	return nil
}

func normaliseLocale(locale string) string {
	if _, ok := localizationData.Messages[locale]; ok {
		return locale
	}
	return defaultLocale
}

func localizationPayload(locale string) LocalizationPayload {
	locale = normaliseLocale(locale)
	locales := append([]LocaleOption(nil), localizationData.Locales...)
	sort.Slice(locales, func(i, j int) bool { return locales[i].Code < locales[j].Code })
	return LocalizationPayload{
		Locale:        locale,
		DefaultLocale: defaultLocale,
		Locales:       locales,
		Messages:      localizationData.Messages[locale],
	}
}

func translate(locale, key string, args ...any) string {
	locale = normaliseLocale(locale)
	message := localizationData.Messages[locale][key]
	if message == "" {
		message = localizationData.Messages[defaultLocale][key]
	}
	if message == "" {
		message = key
	}
	if len(args) > 0 {
		return fmt.Sprintf(message, args...)
	}
	return message
}

type localeState struct {
	mu     sync.RWMutex
	locale string
}

func (s *localeState) set(locale string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.locale = normaliseLocale(locale)
	return s.locale
}

func (s *localeState) get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return normaliseLocale(s.locale)
}
