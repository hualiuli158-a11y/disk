package database

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

const migrationLockName = "disk_schema_migrations"

//go:embed migrations/*.sql
var migrationFiles embed.FS

type Migration struct {
	Version uint64
	Name    string
	UpSQL   string
	DownSQL string
}

type MigrationRunner struct {
	db         *sql.DB
	migrations []Migration
}

func NewMigrationRunner(db *sql.DB) (*MigrationRunner, error) {
	migrations, err := loadMigrations(migrationFiles)
	if err != nil {
		return nil, err
	}
	return &MigrationRunner{db: db, migrations: migrations}, nil
}

func LatestMigrationVersion() (uint64, error) {
	migrations, err := loadMigrations(migrationFiles)
	if err != nil {
		return 0, err
	}
	return migrations[len(migrations)-1].Version, nil
}

func (r *MigrationRunner) Up(ctx context.Context) error {
	return r.withLock(ctx, func(conn *sql.Conn) error {
		version, dirty, err := currentVersion(ctx, conn)
		if err != nil {
			return err
		}
		if dirty {
			return fmt.Errorf("database migration version %d is dirty; repair it before continuing", version)
		}
		for _, migration := range r.migrations {
			if migration.Version <= version {
				continue
			}
			if err := applyMigration(ctx, conn, migration.Version, migration.UpSQL, migration.Version); err != nil {
				return fmt.Errorf("apply migration %06d_%s: %w", migration.Version, migration.Name, err)
			}
			version = migration.Version
		}
		return nil
	})
}

func (r *MigrationRunner) Down(ctx context.Context, steps int) error {
	if steps <= 0 {
		return fmt.Errorf("down steps must be positive")
	}
	return r.withLock(ctx, func(conn *sql.Conn) error {
		version, dirty, err := currentVersion(ctx, conn)
		if err != nil {
			return err
		}
		if dirty {
			return fmt.Errorf("database migration version %d is dirty", version)
		}
		for range steps {
			if version == 0 {
				return nil
			}
			index := sort.Search(len(r.migrations), func(i int) bool { return r.migrations[i].Version >= version })
			if index >= len(r.migrations) || r.migrations[index].Version != version {
				return fmt.Errorf("migration version %d is not embedded in this binary", version)
			}
			previous := uint64(0)
			if index > 0 {
				previous = r.migrations[index-1].Version
			}
			migration := r.migrations[index]
			if err := applyMigration(ctx, conn, migration.Version, migration.DownSQL, previous); err != nil {
				return fmt.Errorf("rollback migration %06d_%s: %w", migration.Version, migration.Name, err)
			}
			version = previous
		}
		return nil
	})
}

func (r *MigrationRunner) Version(ctx context.Context) (uint64, bool, error) {
	var version uint64
	var dirty bool
	err := r.withLock(ctx, func(conn *sql.Conn) error {
		var err error
		version, dirty, err = currentVersion(ctx, conn)
		return err
	})
	return version, dirty, err
}

// Baseline 接管已经由旧版 AutoMigrate 创建的数据库，不执行建表 SQL。
func (r *MigrationRunner) Baseline(ctx context.Context) error {
	baselineVersion := r.migrations[0].Version
	return r.withLock(ctx, func(conn *sql.Conn) error {
		version, dirty, err := currentVersion(ctx, conn)
		if err != nil {
			return err
		}
		if version != 0 && !dirty {
			return fmt.Errorf("database is already at migration version %d", version)
		}
		if err := validateBaselineSchema(ctx, conn); err != nil {
			return err
		}
		return setVersion(ctx, conn, baselineVersion, false)
	})
}

func RequireCurrentVersion(ctx context.Context, db *sql.DB) error {
	latest, err := LatestMigrationVersion()
	if err != nil {
		return err
	}
	var version uint64
	var dirty bool
	err = db.QueryRowContext(ctx, "SELECT version, dirty FROM schema_migrations WHERE id = 1").Scan(&version, &dirty)
	if err != nil {
		return fmt.Errorf("read database migration version: %w; run `go run ./cmd/migrate up` for a new database or `baseline` for an existing database", err)
	}
	if dirty {
		return fmt.Errorf("database migration version %d is dirty", version)
	}
	if version != latest {
		return fmt.Errorf("database migration version is %d, application requires %d; run `go run ./cmd/migrate up`", version, latest)
	}
	return nil
}

func (r *MigrationRunner) withLock(ctx context.Context, fn func(*sql.Conn) error) error {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	var acquired int
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 30)", migrationLockName).Scan(&acquired); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	if acquired != 1 {
		return fmt.Errorf("timed out acquiring migration lock")
	}
	defer func() {
		_, _ = conn.ExecContext(context.WithoutCancel(ctx), "SELECT RELEASE_LOCK(?)", migrationLockName)
	}()

	if err := ensureVersionTable(ctx, conn); err != nil {
		return err
	}
	return fn(conn)
}

func ensureVersionTable(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		id TINYINT UNSIGNED NOT NULL PRIMARY KEY,
		version BIGINT UNSIGNED NOT NULL,
		dirty BOOLEAN NOT NULL
	) ENGINE=InnoDB`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	if _, err := conn.ExecContext(ctx, `INSERT INTO schema_migrations (id, version, dirty)
		VALUES (1, 0, FALSE) ON DUPLICATE KEY UPDATE id = id`); err != nil {
		return fmt.Errorf("initialize schema_migrations: %w", err)
	}
	return nil
}

func currentVersion(ctx context.Context, conn *sql.Conn) (uint64, bool, error) {
	var version uint64
	var dirty bool
	if err := conn.QueryRowContext(ctx, "SELECT version, dirty FROM schema_migrations WHERE id = 1").Scan(&version, &dirty); err != nil {
		return 0, false, err
	}
	return version, dirty, nil
}

func setVersion(ctx context.Context, conn *sql.Conn, version uint64, dirty bool) error {
	_, err := conn.ExecContext(ctx, "UPDATE schema_migrations SET version = ?, dirty = ? WHERE id = 1", version, dirty)
	return err
}

func applyMigration(ctx context.Context, conn *sql.Conn, dirtyVersion uint64, source string, finalVersion uint64) error {
	if err := setVersion(ctx, conn, dirtyVersion, true); err != nil {
		return err
	}
	statements, err := splitSQLStatements(source)
	if err != nil {
		return err
	}
	for _, statement := range statements {
		if _, err := conn.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	return setVersion(ctx, conn, finalVersion, false)
}

func loadMigrations(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, "migrations")
	if err != nil {
		return nil, err
	}
	type parts struct{ up, down string }
	byVersion := make(map[uint64]*parts)
	names := make(map[uint64]string)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		segments := strings.Split(entry.Name(), ".")
		if len(segments) != 3 || (segments[1] != "up" && segments[1] != "down") {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		prefix := strings.SplitN(segments[0], "_", 2)
		if len(prefix) != 2 {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.ParseUint(prefix[0], 10, 64)
		if err != nil || version == 0 {
			return nil, fmt.Errorf("invalid migration version in %q", entry.Name())
		}
		content, err := fs.ReadFile(fsys, "migrations/"+entry.Name())
		if err != nil {
			return nil, err
		}
		if byVersion[version] == nil {
			byVersion[version] = &parts{}
		}
		if segments[1] == "up" {
			byVersion[version].up = string(content)
		} else {
			byVersion[version].down = string(content)
		}
		names[version] = prefix[1]
	}

	versions := make([]uint64, 0, len(byVersion))
	for version := range byVersion {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return versions[i] < versions[j] })
	migrations := make([]Migration, 0, len(versions))
	for _, version := range versions {
		part := byVersion[version]
		if strings.TrimSpace(part.up) == "" || strings.TrimSpace(part.down) == "" {
			return nil, fmt.Errorf("migration %d must have both up and down files", version)
		}
		migrations = append(migrations, Migration{Version: version, Name: names[version], UpSQL: part.up, DownSQL: part.down})
	}
	if len(migrations) == 0 {
		return nil, errors.New("no migrations found")
	}
	return migrations, nil
}

func splitSQLStatements(source string) ([]string, error) {
	var statements []string
	var builder strings.Builder
	var quote rune
	escaped := false
	runes := []rune(source)
	for i := 0; i < len(runes); i++ {
		character := runes[i]
		if quote == 0 && character == '-' && i+1 < len(runes) && runes[i+1] == '-' {
			for i < len(runes) && runes[i] != '\n' {
				i++
			}
			builder.WriteRune('\n')
			continue
		}
		if quote != 0 {
			builder.WriteRune(character)
			if escaped {
				escaped = false
				continue
			}
			if character == '\\' {
				escaped = true
				continue
			}
			if character == quote {
				quote = 0
			}
			continue
		}
		switch character {
		case '\'', '"', '`':
			quote = character
			builder.WriteRune(character)
		case ';':
			if statement := strings.TrimSpace(builder.String()); statement != "" {
				statements = append(statements, statement)
			}
			builder.Reset()
		default:
			builder.WriteRune(character)
		}
	}
	if quote != 0 {
		return nil, errors.New("unterminated SQL quote")
	}
	if statement := strings.TrimSpace(builder.String()); statement != "" {
		statements = append(statements, statement)
	}
	return statements, nil
}

func validateBaselineSchema(ctx context.Context, conn *sql.Conn) error {
	requiredTables := []string{"users", "folders", "physical_files", "files", "trash_entries"}
	for _, table := range requiredTables {
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables
			WHERE table_schema = DATABASE() AND table_name = ?`, table).Scan(&count); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("cannot baseline: required table %s is missing", table)
		}
	}
	requiredColumns := map[string][]string{
		"users":          {"used_quota", "quota_limit"},
		"folders":        {"deletion_key", "trash_id", "deleted_at"},
		"files":          {"physical_file_id", "deletion_key", "trash_id", "deleted_at"},
		"physical_files": {"sha256", "size", "storage_key"},
		"trash_entries":  {"resource_type", "resource_id", "trashed_at", "expires_at"},
	}
	for table, columns := range requiredColumns {
		for _, column := range columns {
			var count int
			if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
				WHERE table_schema = DATABASE() AND table_name = ? AND column_name = ?`, table, column).Scan(&count); err != nil {
				return err
			}
			if count != 1 {
				return fmt.Errorf("cannot baseline: required column %s.%s is missing", table, column)
			}
		}
	}
	for _, index := range []struct{ table, name string }{
		{table: "folders", name: "idx_active_folder_name"},
		{table: "files", name: "idx_active_file_name"},
	} {
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
			WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`, index.table, index.name).Scan(&count); err != nil {
			return err
		}
		if count == 0 {
			return fmt.Errorf("cannot baseline: required index %s on %s is missing", index.name, index.table)
		}
	}
	for _, legacy := range []struct{ table, name string }{
		{table: "folders", name: "idx_user_parent_folder_name"},
		{table: "files", name: "idx_user_folder_file_name"},
	} {
		var count int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics
			WHERE table_schema = DATABASE() AND table_name = ? AND index_name = ?`, legacy.table, legacy.name).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("cannot baseline: legacy index %s on %s must be removed", legacy.name, legacy.table)
		}
	}
	return nil
}
