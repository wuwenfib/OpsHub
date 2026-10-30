//go:build integration

package service

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/fh/opshub/backend/internal/apperr"
	"github.com/fh/opshub/backend/internal/cmdb"
	"github.com/fh/opshub/backend/internal/store"
)

// newTestStore 连接 opshub_test 库、迁移到最新，并清空 CMDB 表。需先 make up。
func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	url := os.Getenv("OPSHUB_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://opshub:opshub@localhost:5432/opshub_test?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := store.Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := store.Migrate(url); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := pool.Exec(ctx, "TRUNCATE vendor, business_system, host RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	return store.NewStore(pool)
}

// must 用于测试准备步骤：出错直接 panic（测试框架会把它记为失败）。
// 不接收 t，是因为 Go 只有在 f() 作为唯一实参时才展开多返回值，must(f()) 编译不过。
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func wantErr(t *testing.T, err, target error) {
	t.Helper()
	if !errors.Is(err, target) {
		t.Fatalf("want %v, got %v", target, err)
	}
}

func TestCMDBSystemDetail(t *testing.T) {
	ctx := context.Background()
	svc := NewCMDB(newTestStore(t))

	v := must(svc.CreateVendor(ctx, cmdb.VendorInput{Name: "星河软件", ContractEnd: "2027-06-30"}))
	must(svc.CreateSystem(ctx, cmdb.SystemInput{Code: "GOV-AUTH", Name: "统一身份认证", Level: 1}))
	must(svc.CreateSystem(ctx, cmdb.SystemInput{Code: "GOV-APPLY", Name: "一网通办", Level: 1, VendorID: &v.ID}))
	h1 := must(svc.CreateHost(ctx, cmdb.HostInput{IP: "10.10.1.11", Hostname: "apply-app01", Env: "prod"}))
	h2 := must(svc.CreateHost(ctx, cmdb.HostInput{IP: "10.10.1.12", Hostname: "apply-app02", Env: "prod"}))

	must(svc.SetSystemHosts(ctx, "GOV-APPLY", []int64{h1.ID, h2.ID}))
	d := must(svc.SetSystemDependencies(ctx, "GOV-APPLY", []string{"GOV-AUTH"}))

	if d.Vendor == nil || d.Vendor.Name != "星河软件" {
		t.Errorf("vendor = %+v", d.Vendor)
	}
	if len(d.Hosts) != 2 || len(d.DependsOn) != 1 || d.DependsOn[0].Code != "GOV-AUTH" {
		t.Errorf("hosts = %d, depends_on = %+v", len(d.Hosts), d.DependsOn)
	}
	auth := must(svc.GetSystem(ctx, "GOV-AUTH"))
	if len(auth.DependedBy) != 1 || auth.DependedBy[0].Code != "GOV-APPLY" {
		t.Errorf("GOV-AUTH depended_by = %+v", auth.DependedBy)
	}

	// 删除厂商后系统仍在，只是 vendor 置空。
	if err := svc.DeleteVendor(ctx, v.ID); err != nil {
		t.Fatal(err)
	}
	if d := must(svc.GetSystem(ctx, "GOV-APPLY")); d.Vendor != nil {
		t.Errorf("vendor should be nil after delete, got %+v", d.Vendor)
	}
}

func TestCMDBErrors(t *testing.T) {
	ctx := context.Background()
	svc := NewCMDB(newTestStore(t))
	must(svc.CreateSystem(ctx, cmdb.SystemInput{Code: "GOV-AUTH", Name: "统一身份认证", Level: 1}))
	must(svc.CreateSystem(ctx, cmdb.SystemInput{Code: "GOV-APPLY", Name: "一网通办", Level: 1}))
	h := must(svc.CreateHost(ctx, cmdb.HostInput{IP: "10.10.1.11", Hostname: "apply-app01", Env: "prod"}))

	t.Run("duplicate code", func(t *testing.T) {
		_, err := svc.CreateSystem(ctx, cmdb.SystemInput{Code: "GOV-AUTH", Name: "x", Level: 2})
		wantErr(t, err, apperr.ErrConflict)
	})
	t.Run("duplicate ip", func(t *testing.T) {
		_, err := svc.CreateHost(ctx, cmdb.HostInput{IP: "10.10.1.11", Hostname: "other", Env: "prod"})
		wantErr(t, err, apperr.ErrConflict)
	})
	t.Run("unknown vendor", func(t *testing.T) {
		missing := int64(999)
		_, err := svc.CreateSystem(ctx, cmdb.SystemInput{Code: "GOV-X", Name: "x", Level: 2, VendorID: &missing})
		wantErr(t, err, apperr.ErrValidation)
	})
	t.Run("get missing", func(t *testing.T) {
		_, err := svc.GetSystem(ctx, "GOV-NOPE")
		wantErr(t, err, apperr.ErrNotFound)
	})
	t.Run("delete missing", func(t *testing.T) {
		wantErr(t, svc.DeleteHost(ctx, 999), apperr.ErrNotFound)
	})

	// 关系替换失败时事务回滚，原关系保持不变。
	must(svc.SetSystemDependencies(ctx, "GOV-APPLY", []string{"GOV-AUTH"}))
	must(svc.SetSystemHosts(ctx, "GOV-APPLY", []int64{h.ID}))

	t.Run("unknown dependency rolls back", func(t *testing.T) {
		_, err := svc.SetSystemDependencies(ctx, "GOV-APPLY", []string{"GOV-NOPE1", "GOV-NOPE2"})
		wantErr(t, err, apperr.ErrValidation)
		if got := err.Error(); got != "validation failed: 依赖的系统不存在: GOV-NOPE1, GOV-NOPE2" {
			t.Errorf("message = %q", got)
		}
		d := must(svc.GetSystem(ctx, "GOV-APPLY"))
		if len(d.DependsOn) != 1 || d.DependsOn[0].Code != "GOV-AUTH" {
			t.Errorf("dependencies changed after failed update: %+v", d.DependsOn)
		}
	})
	t.Run("unknown host rolls back", func(t *testing.T) {
		_, err := svc.SetSystemHosts(ctx, "GOV-APPLY", []int64{999})
		wantErr(t, err, apperr.ErrValidation)
		if d := must(svc.GetSystem(ctx, "GOV-APPLY")); len(d.Hosts) != 1 {
			t.Errorf("hosts changed after failed update: %+v", d.Hosts)
		}
	})
}
