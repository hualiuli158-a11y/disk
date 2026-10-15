package repository

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

var repositoryTestDB *gorm.DB

func TestMain(m *testing.M) {
	os.Exit(runRepositoryTests(m))
}

func runRepositoryTests(m *testing.M) (code int) {
	// 保留现有约定：普通 go test 不启动 Docker。
	if os.Getenv("DISK_MYSQL_INTEGRATION") != "1" {
		return m.Run()
	}

	ctx := context.Background()

	//1.创建容器，Context，镜像及版本，配置
	container, err := tcMySQL.Run(ctx, "mysql:8.0.36",
		tcMySQL.WithDatabase("disk_repository_test"),
		tcMySQL.WithUsername("test"),
		tcMySQL.WithPassword("test123456"),
	)

	if err != nil {
		if container != nil {
			_ = testcontainers.TerminateContainer(container)
		}
		log.Printf("启动测试 MySQL 失败: %v", err)
		return 1
	}

	//测试结束删除容器
	defer func() {
		if err := testcontainers.TerminateContainer(container); err != nil {
			log.Printf("销毁测试 MySQL 失败: %v", err)
			code = 1
		}
	}()

	dsn, err := container.ConnectionString(ctx)
	if err != nil {
		log.Printf("获取 MySQL 连接地址失败: %v", err)
		return 1
	}

	//解析DSN
	parsed, err := driverMySQL.ParseDSN(dsn)
	if err != nil || parsed.DBName != "disk_repository_test" {
		log.Printf("测试数据库连接信息无效: %v", err)
		return 1
	}

	//连接数据库
	db, err := database.OpenMySQL(setting.DatabaseConfig{
		Host:         parsed.Addr,
		Username:     parsed.User,
		Password:     parsed.Passwd,
		DatabaseName: parsed.DBName,
	})

	if err != nil {
		log.Printf("连接测试 MySQL 失败: %v", err)
		return 1
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Printf("获取数据库连接池失败: %v", err)
		return 1
	}
	defer sqlDB.Close()

	runner, err := database.NewMigrationRunner(sqlDB)
	if err != nil {
		log.Printf("加载迁移失败: %v", err)
		return 1
	}
	if err := runner.Up(ctx); err != nil {
		log.Printf("迁移测试数据库失败: %v", err)
		return 1
	}

	repositoryTestDB = db
	return m.Run()
}
