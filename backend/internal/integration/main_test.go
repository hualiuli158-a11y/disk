package integration

import (
	"context"
	"log"
	"os"
	"testing"

	"disk/internal/database"
	"disk/pkg/setting"

	driverMySQL "github.com/go-sql-driver/mysql"
	"github.com/testcontainers/testcontainers-go"
	tcMySQL "github.com/testcontainers/testcontainers-go/modules/mysql"
	"gorm.io/gorm"
)

var fileServiceTestDB *gorm.DB

func TestMain(m *testing.M) {
	os.Exit(runFileServiceTests(m))
}

func runFileServiceTests(m *testing.M) (code int) {
	if os.Getenv("DISK_MYSQL_INTEGRATION") != "1" {
		return m.Run()
	}

	ctx := context.Background()
	const databaseName = "disk_file_service_test"
	container, err := tcMySQL.Run(ctx, "mysql:8.0.36",
		tcMySQL.WithDatabase(databaseName),
		tcMySQL.WithUsername("test"),
		tcMySQL.WithPassword("test123456"),
	)
	if err != nil {
		if container != nil {
			_ = testcontainers.TerminateContainer(container)
		}
		log.Printf("start file service test MySQL: %v", err)
		return 1
	}
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			log.Printf("terminate file service test MySQL: %v", err)
			code = 1
		}
	}()

	dsn, err := container.ConnectionString(ctx)
	if err != nil {
		log.Printf("get file service test MySQL DSN: %v", err)
		return 1
	}
	parsed, err := driverMySQL.ParseDSN(dsn)
	if err != nil || parsed.DBName != databaseName {
		log.Printf("invalid file service test MySQL DSN: %v", err)
		return 1
	}
	db, err := database.OpenMySQL(setting.DatabaseConfig{
		Host:         parsed.Addr,
		Username:     parsed.User,
		Password:     parsed.Passwd,
		DatabaseName: parsed.DBName,
	})
	if err != nil {
		log.Printf("connect file service test MySQL: %v", err)
		return 1
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Printf("get file service test MySQL pool: %v", err)
		return 1
	}
	defer sqlDB.Close()

	runner, err := database.NewMigrationRunner(sqlDB)
	if err != nil {
		log.Printf("load file service test migrations: %v", err)
		return 1
	}
	if err := runner.Up(ctx); err != nil {
		log.Printf("migrate file service test MySQL: %v", err)
		return 1
	}
	fileServiceTestDB = db
	return m.Run()
}
