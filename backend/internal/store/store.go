package store

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fh/opshub/backend/internal/apperr"
)

// Store 把连接池和 sqlc 生成的 Queries 组合在一起。
// 直接调用 s.GetVendor(...) 等方法走连接池；需要事务时用 InTx。
type Store struct {
	*Queries
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{Queries: New(pool), pool: pool}
}

// InTx 在一个事务里执行 fn：fn 返回 nil 则提交，返回错误或 panic 则回滚。
// 作用类似 C++ 里的 RAII 事务守卫。
func (s *Store) InTx(ctx context.Context, fn func(q *Queries) error) error {
	return pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		return fn(s.Queries.WithTx(tx))
	})
}

// PostgreSQL 错误码，见 https://www.postgresql.org/docs/16/errcodes-appendix.html
const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
	pgCheckViolation      = "23514"
)

// MapError 把数据库层的错误翻译成 apperr 里的业务错误；无法识别的原样返回（最终成为 500）。
// what 用于拼出可读的错误信息，例如 "system GOV-APPLY"。
func MapError(err error, what string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: %s", apperr.ErrNotFound, what)
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case pgUniqueViolation:
			return fmt.Errorf("%w: %s 已存在", apperr.ErrConflict, what)
		case pgForeignKeyViolation:
			return fmt.Errorf("%w: %s 引用的对象不存在或仍被引用（%s）", apperr.ErrValidation, what, pgErr.ConstraintName)
		case pgCheckViolation:
			return fmt.Errorf("%w: %s 不满足约束 %s", apperr.ErrValidation, what, pgErr.ConstraintName)
		}
	}
	return err
}
