package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileAtomicLeavesNoTempFileAfterFailure(t *testing.T) {
	for _, name := range []string{"missing-dir/file", "target"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, name)
			if name == "target" {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			if err := writeFileAtomic(path, []byte("new"), 0o600); err == nil {
				t.Fatal("writeFileAtomic unexpectedly succeeded")
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if filepath.Ext(entry.Name()) == ".tmp" {
					t.Errorf("temporary file remains after failed write: %s", entry.Name())
				}
			}
		})
	}
}
