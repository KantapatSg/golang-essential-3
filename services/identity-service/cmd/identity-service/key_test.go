package main

import (
	"path/filepath"
	"testing"
)

func TestLoadKeysRequiresFilesOutsideDev(t *testing.T) {
	t.Setenv("DEV_MODE", "false")
	_, err := loadKeys(filepath.Join(t.TempDir(), "missing.pem"), filepath.Join(t.TempDir(), "public.pem"))
	if err == nil {
		t.Fatal("expected production key loading to fail when the secret is missing")
	}
}

func TestLoadKeysCreatesDevPair(t *testing.T) {
	t.Setenv("DEV_MODE", "true")
	dir := t.TempDir()
	priv := filepath.Join(dir, "private.pem")
	pub := filepath.Join(dir, "public.pem")
	if _, err := loadKeys(priv, pub); err != nil {
		t.Fatalf("expected DEV_MODE key generation: %v", err)
	}
}
