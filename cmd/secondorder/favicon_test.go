package main

import (
	"io/fs"
	"testing"
)

func TestStaticFS(t *testing.T) {
	// Check if static directory is embedded
	entries, err := fs.ReadDir(staticFS, "static")
	if err != nil {
		t.Fatalf("failed to read static directory from embed.FS: %v", err)
	}

	foundFavicon := false
	for _, entry := range entries {
		if entry.Name() == "favicon.svg" {
			foundFavicon = true
			break
		}
	}

	if !foundFavicon {
		t.Error("favicon.svg not found in embedded static directory")
	}
}
