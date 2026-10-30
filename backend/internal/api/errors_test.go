package api

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/fh/opshub/backend/internal/apperr"
)

func TestStatusOf(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{fmt.Errorf("%w: system GOV-X", apperr.ErrNotFound), http.StatusNotFound, "not_found"},
		{fmt.Errorf("%w: level", apperr.ErrValidation), http.StatusBadRequest, "validation"},
		{fmt.Errorf("%w: code", apperr.ErrConflict), http.StatusConflict, "conflict"},
		{fmt.Errorf("%w: closed -> ack", apperr.ErrInvalidTransition), http.StatusConflict, "invalid_transition"},
		// 多层包装后仍能识别
		{fmt.Errorf("set hosts: %w", fmt.Errorf("%w: host", apperr.ErrValidation)), http.StatusBadRequest, "validation"},
		{errors.New("connection reset"), http.StatusInternalServerError, "internal"},
	}
	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			status, code := statusOf(tt.err)
			if status != tt.wantStatus || code != tt.wantCode {
				t.Errorf("got (%d, %s), want (%d, %s)", status, code, tt.wantStatus, tt.wantCode)
			}
		})
	}
}
