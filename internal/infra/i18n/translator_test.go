//go:build !integration

package i18n_test

import (
	"telegram-ai-subscription/internal/infra/i18n"
	"testing"
	"testing/fstest"
)

func TestTranslator(t *testing.T) {
	t.Run("should load and translate keys correctly", func(t *testing.T) {
		// Arrange: Create an in-memory virtual filesystem for the test.
		testFS := fstest.MapFS{
			"locales/fa.yaml": {
				Data: []byte("greeting: سلام\nwelcome_user: سلام %s"),
			},
			"locales/policy-fa.txt": {
				Data: []byte("Test Policy"),
			},
		}

		// Act: Create the translator using the virtual filesystem.
		translator, err := i18n.NewTranslator(testFS, "fa")
		if err != nil {
			t.Fatalf("NewTranslator failed: %v", err)
		}

		// Assert
		if got := translator.T("greeting"); got != "سلام" {
			t.Errorf("expected 'سلام', got '%s'", got)
		}
		if got := translator.T("welcome_user", "Ali"); got != "سلام Ali" {
			t.Errorf("expected 'سلام Ali', got '%s'", got)
		}
		if got := translator.Policy(); got != "Test Policy" {
			t.Errorf("expected 'Test Policy', got '%s'", got)
		}
	})
}
