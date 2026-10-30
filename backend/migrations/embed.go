// Package migrations 把迁移 SQL 编译进二进制，启动时由 store.Migrate 执行。
package migrations

import "embed"

//go:embed *.sql
var FS embed.FS
