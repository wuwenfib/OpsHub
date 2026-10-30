//go:build integration

package store

import (
	"context"
	"os"
	"testing"
)

// testDatabaseURL 指向 compose 中 PostgreSQL 的 opshub_test 库（需先 make up）。
func testDatabaseURL() string {
	if v := os.Getenv("OPSHUB_TEST_DATABASE_URL"); v != "" {
		return v
	}
	return "postgres://opshub:opshub@localhost:5432/opshub_test?sslmode=disable"
}

func TestMigrateCreatesTablesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	url := testDatabaseURL()

	pool, err := Connect(ctx, url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	// 跑两次：第二次应当是"已是最新"，不报错。
	for i := 0; i < 2; i++ {
		if err := Migrate(url); err != nil {
			t.Fatalf("migrate run %d: %v", i+1, err)
		}
	}

	for _, table := range []string{"vendor", "business_system", "host", "system_host", "system_dependency"} {
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists); err != nil {
			t.Fatalf("check %s: %v", table, err)
		}
		if !exists {
			t.Errorf("table %s not created", table)
		}
	}
}
