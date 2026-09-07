package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"
)

const defaultLanguage = "sv"

type Localized map[string]string

func (text Localized) Default() string { return text[defaultLanguage] }
func (text Localized) JSON() string    { data, _ := json.Marshal(text); return string(data) }

type Catalog map[string]map[string]string

func loadCatalog(dir string) (Catalog, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	catalog := Catalog{}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var messages map[string]string
		if err := json.Unmarshal(data, &messages); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if messages["languageName"] == "" {
			return nil, fmt.Errorf("%s: missing languageName", path)
		}
		catalog[strings.TrimSuffix(filepath.Base(path), ".json")] = messages
	}
	if len(catalog[defaultLanguage]) == 0 {
		return nil, fmt.Errorf("missing default locale %s", defaultLanguage)
	}
	// Missing translations fall back to the default language, in both Go and JS.
	for _, messages := range catalog {
		for key, value := range catalog[defaultLanguage] {
			if messages[key] == "" {
				messages[key] = value
			}
		}
	}
	return catalog, nil
}

func (catalog Catalog) templateFuncs() template.FuncMap {
	lookup := func(key string) (string, error) {
		value, ok := catalog[defaultLanguage][key]
		if !ok {
			return "", fmt.Errorf("unknown translation key %q", key)
		}
		return value, nil
	}
	return template.FuncMap{
		"t": lookup,
		// Only this helper creates markup; keys and translated text are always escaped.
		"msg": func(key string) (template.HTML, error) {
			value, err := lookup(key)
			if err != nil {
				return "", err
			}
			return template.HTML(`<span data-i18n="` + template.HTMLEscapeString(key) + `">` + template.HTMLEscapeString(value) + `</span>`), nil
		},
	}
}
