package i18n

import (
	"embed"
	"fmt"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

//go:embed locales
var localesFS embed.FS

type Translator interface {
	T(key string, args ...interface{}) string
}

type translator struct {
	translations map[string]string
}

// newTranslatorFromBytes is an internal constructor that parses raw YAML data.
// This allows us to create a translator from any source, making testing easy.
func newTranslatorFromBytes(data []byte) (Translator, error) {
	var translations map[string]string
	if err := yaml.Unmarshal(data, &translations); err != nil {
		return nil, fmt.Errorf("failed to parse translation data: %w", err)
	}
	return &translator{translations: translations}, nil
}

// NewTranslator loads a specific language from the embedded filesystem for production use.
func NewTranslator(langCode string) (Translator, error) {
	filePath := filepath.Join("locales", fmt.Sprintf("%s.yaml", langCode))
	data, err := localesFS.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded translation file %s: %w", filePath, err)
	}
	// Call the internal constructor with the data from the embedded file.
	return newTranslatorFromBytes(data)
}

// T (Translate) function remains the same.
func (t *translator) T(key string, args ...interface{}) string {
	format, ok := t.translations[key]
	if !ok {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(format, args...)
	}
	return format
}
