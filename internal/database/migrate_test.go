package database

import (
	"testing"
	"testing/fstest"
)

func TestLoadMigrations(t *testing.T) {
	files := fstest.MapFS{
		"migrations/000002_second.up.sql":   {Data: []byte("SELECT 2;")},
		"migrations/000002_second.down.sql": {Data: []byte("SELECT -2;")},
		"migrations/000001_first.up.sql":    {Data: []byte("SELECT 1;")},
		"migrations/000001_first.down.sql":  {Data: []byte("SELECT -1;")},
	}
	migrations, err := loadMigrations(files)
	if err != nil {
		t.Fatalf("load migrations: %v", err)
	}
	if len(migrations) != 2 || migrations[0].Version != 1 || migrations[0].Name != "first" || migrations[1].Version != 2 {
		t.Fatalf("unexpected migrations: %+v", migrations)
	}
}

func TestSplitSQLStatements(t *testing.T) {
	source := `
-- comment with a semicolon;
CREATE TABLE example (value VARCHAR(20) DEFAULT ';');
INSERT INTO example(value) VALUES ('a; b');
`
	statements, err := splitSQLStatements(source)
	if err != nil {
		t.Fatalf("split SQL: %v", err)
	}
	if len(statements) != 2 {
		t.Fatalf("statement count = %d, want 2: %#v", len(statements), statements)
	}
}

func TestEmbeddedMigrationVersion(t *testing.T) {
	version, err := LatestMigrationVersion()
	if err != nil {
		t.Fatalf("latest version: %v", err)
	}
	if version != 1 {
		t.Fatalf("latest version = %d, want 1", version)
	}
}
