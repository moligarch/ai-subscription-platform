package i18n

import (
	"embed"
	"fmt"
	"io/fs" // <-- This package contains the correct ReadFile function
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed locales
var LocalesFS embed.FS

// Translator interface and struct are unchanged...
type Translator struct {
	translations map[string]string
	policyText   string
}

// NewTranslator is now more flexible and testable.
func NewTranslator(fsys fs.FS, langCode string) (*Translator, error) {
	filePath := filepath.Join("locales", fmt.Sprintf("%s.yaml", langCode))

	// Use the fs.ReadFile function, which works with any fs.FS interface.
	data, err := fs.ReadFile(fsys, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read translation file %s: %w", filePath, err)
	}

	var translations map[string]string
	if err := yaml.Unmarshal(data, &translations); err != nil {
		return nil, fmt.Errorf("failed to parse translation file: %w", err)
	}

	// Incorporating your improvement for language-specific policy files.
	policyPath := filepath.Join("locales", fmt.Sprintf("policy-%s.txt", langCode))

	// Use the fs.ReadFile function here as well.
	policyBytes, err := fs.ReadFile(fsys, policyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read policy file %s: %w", policyPath, err)
	}

	return &Translator{
		translations: translations,
		policyText:   string(policyBytes),
	}, nil
}

// T (Translate) function remains the same.
func (t *Translator) T(key string, args ...interface{}) string {
	format, ok := t.translations[key]
	if !ok {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(format, args...)
	}
	return format
}

func (t *Translator) Policy() string {
	return t.policyText
}
