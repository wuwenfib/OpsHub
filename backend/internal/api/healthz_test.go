package api

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func TestHealthz(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		pingErr    error
		wantStatus int
	}{
		{"db ok", nil, http.StatusOK},
		{"db down", errors.New("connection refused"), http.StatusServiceUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			NewRouter(fakePinger{tt.pingErr}).ServeHTTP(rec, httptest.NewRequest("GET", "/healthz", nil))
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d, body = %s", rec.Code, tt.wantStatus, rec.Body)
			}
		})
	}
}
