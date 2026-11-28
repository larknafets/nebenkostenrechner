// Package storetest gives tests a ready database fast.
//
// A fresh store.Open per test costs the schema, the migrations and the seed
// data, and every write waits for the disk. With hundreds of tests per package
// that dominates the run time, most of all on a slow disk. Opener migrates once
// per test binary, keeps the bytes of the finished file and writes them into a
// new t.TempDir() for every call.
//
// The package does not import store: the opening function is passed in, so the
// tests of package store can use it without an import cycle.
package storetest

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// Opener returns a function that gives each test its own database file,
// identical to what open creates for a new path. open runs once for the
// template and then once per call, for the copy (a cheap open of a finished
// file). The database is closed when the test ends.
func Opener(open func(path string) (*sql.DB, error)) func(testing.TB) *sql.DB {
	var (
		once     sync.Once
		template []byte
		initErr  error
	)
	return func(t testing.TB) *sql.DB {
		t.Helper()
		once.Do(func() { template, initErr = build(open) })
		if initErr != nil {
			t.Fatalf("template database: %v", initErr)
		}
		path := filepath.Join(t.TempDir(), "test.db")
		if err := os.WriteFile(path, template, 0o600); err != nil {
			t.Fatalf("copy template database: %v", err)
		}
		db, err := open(path)
		if err != nil {
			t.Fatalf("open store: %v", err)
		}
		t.Cleanup(func() { db.Close() })
		return db
	}
}

// build creates the template file, closes it (so the file is complete, no
// journal left) and returns its bytes. The file is removed again: nothing
// outlives the test binary.
func build(open func(path string) (*sql.DB, error)) ([]byte, error) {
	dir, err := os.MkdirTemp("", "nk-template-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }() // a leftover temp folder must not fail a test
	path := filepath.Join(dir, "template.db")
	db, err := open(path)
	if err != nil {
		return nil, err
	}
	if err := db.Close(); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}
