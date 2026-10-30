// Package apperr 定义跨层共用的哨兵错误。领域层和 service 层用 fmt.Errorf("...: %w", ErrX)
// 包上具体原因，api 层用 errors.Is 识别并映射成 HTTP 状态码。
package apperr

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrValidation        = errors.New("validation failed")
	ErrConflict          = errors.New("conflict")
	ErrInvalidTransition = errors.New("invalid state transition")
)
