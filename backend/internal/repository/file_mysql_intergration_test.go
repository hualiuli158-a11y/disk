package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"disk/internal/model"

	"gorm.io/gorm"
)

func openRepositoryTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	if os.Getenv("DISK_MYSQL_INTEGRATION") != "1" {
		t.Skip("设置 DISK_MYSQL_INTEGRATION=1 后运行 MySQL 集成测试")
	}
	if repositoryTestDB == nil {
		t.Fatal("TestMain 未初始化测试 MySQL")
	}

	return repositoryTestDB
}

func TestFilesRepositoryCreateUploadedFileAfterUserRegistration(t *testing.T) {
	db := openRepositoryTestDB(t)

	// 使用事务隔离当前测试，测试结束后自动回滚。
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("开启测试事务失败: %v", tx.Error)
	}
	t.Cleanup(func() {
		_ = tx.Rollback().Error
	})

	ctx := context.Background()

	users := NewUserRepository(tx)
	files := NewFilesRepository(tx)

	user := &model.User{
		Username: fmt.Sprintf(
			"file_repository_test_%d",
			time.Now().UnixNano(),
		),
		Password:   "integration-test-password-hash",
		QuotaLimit: 1024,
	}

	if err := users.CreateRoot(ctx, user); err != nil {
		t.Fatalf("创建测试用户和根目录失败: %v", err)
	}

	var root model.Folder
	if err := tx.
		Where("user_id = ? AND parent_id IS NULL", user.ID).
		First(&root).Error; err != nil {
		t.Fatalf("查询测试根目录失败: %v", err)
	}

	const (
		fileName    = "hello.txt"
		fileSize    = int64(5)
		sha256Value = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" +
			"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		storageKey = "objects/test/create-uploaded-file"
	)

	logical, physical, physicalCreated, err :=
		files.CreateUploadedFile(
			ctx,
			user.ID,
			root.ID,
			fileName,
			sha256Value,
			fileSize,
			storageKey,
		)
	if err != nil {
		t.Fatalf("创建上传文件失败: %v", err)
	}

	if logical == nil {
		t.Fatal("没有返回逻辑文件")
	}
	if physical == nil {
		t.Fatal("没有返回物理文件")
	}
	if !physicalCreated {
		t.Fatal("第一次上传应该创建物理文件")
	}

	if logical.Name != fileName {
		t.Fatalf(
			"文件名错误：得到 %q，期望 %q",
			logical.Name,
			fileName,
		)
	}
	if physical.Size != fileSize {
		t.Fatalf(
			"物理文件大小错误：得到 %d，期望 %d",
			physical.Size,
			fileSize,
		)
	}

	// 直接查询真实 MySQL，验证事务执行后的数据库状态。
	var physicalCount int64
	if err := tx.Model(&model.PhysicalFile{}).
		Where("sha256 = ?", sha256Value).
		Count(&physicalCount).Error; err != nil {
		t.Fatalf("统计物理文件失败: %v", err)
	}
	if physicalCount != 1 {
		t.Fatalf(
			"物理文件数量错误：得到 %d，期望 1",
			physicalCount,
		)
	}

	var savedUser model.User
	if err := tx.First(&savedUser, user.ID).Error; err != nil {
		t.Fatalf("重新查询用户失败: %v", err)
	}
	if savedUser.UsedQuota != fileSize {
		t.Fatalf(
			"用户额度错误：得到 %d，期望 %d",
			savedUser.UsedQuota,
			fileSize,
		)
	}
}
