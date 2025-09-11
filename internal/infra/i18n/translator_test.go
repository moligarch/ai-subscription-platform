//go:build !integration

package i18n

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTranslator(t *testing.T) {
	// 1. Arrange: Create a temporary YAML file with test data.
	tempDir := t.TempDir()
	filePath := filepath.Join(tempDir, "test_fa.yaml")
	contentBytes := []byte("greeting: سلام\nwelcome_user: سلام %s")
	if err := os.WriteFile(filePath, contentBytes, 0644); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	// 2. Act: The test now reads its own temporary file and uses the internal constructor.
	// It no longer calls the production NewTranslator, avoiding the embed conflict.
	translator, err := newTranslatorFromBytes(contentBytes)
	if err != nil {
		t.Fatalf("newTranslatorFromBytes failed: %v", err)
	}

	// 3. Assert
	t.Run("should translate a simple key", func(t *testing.T) {
		got := translator.T("greeting")
		want := "سلام"
		if got != want {
			t.Errorf("wanted '%s', got '%s'", want, got)
		}
	})

	t.Run("should return key if not found", func(t *testing.T) {
		got := translator.T("nonexistent_key")
		want := "nonexistent_key"
		if got != want {
			t.Errorf("wanted '%s', got '%s'", want, got)
		}
	})

	t.Run("should format arguments correctly", func(t *testing.T) {
		got := translator.T("welcome_user", "Ali")
		want := "سلام Ali"
		if got != want {
			t.Errorf("wanted '%s', got '%s'", want, got)
		}
	})
}
