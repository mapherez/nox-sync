package storage

import (
	"strings"
	"testing"
)

func FuzzNormalizeVaultPath(f *testing.F) {
	f.Add("notes/test.md")
	f.Add("folder/subfolder/file.md")
	f.Add("../secret.txt")
	f.Add("/absolute/path.md")
	f.Add("..")
	f.Add(".")
	f.Add("")
	f.Add(`folder\windows\path.md`)
	f.Add("folder/../../escape.md")
	f.Add(strings.Repeat("a", 4096) + ".md")

	f.Fuzz(func(t *testing.T, input string) {
		normalized, err := NormalizeVaultPath(input)

		if err != nil {
			return
		}

		if normalized == "" {
			t.Fatalf("normalized path should not be empty")
		}

		if strings.HasPrefix(normalized, "/") {
			t.Fatalf("normalized path should not be absolute: %q", normalized)
		}

		if normalized == ".." || strings.HasPrefix(normalized, "../") || strings.Contains(normalized, "/../") {
			t.Fatalf("normalized path escapes vault root: %q", normalized)
		}

		if strings.Contains(normalized, "\\") {
			t.Fatalf("normalized path should not contain backslashes: %q", normalized)
		}
	})
}
