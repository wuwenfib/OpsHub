package sla

import (
	"errors"
	"testing"
	"time"

	"github.com/fh/opshub/backend/internal/apperr"
)

func TestParseSeverity(t *testing.T) {
	tests := []struct {
		in      string
		want    Severity
		wantErr bool
	}{
		{"P1", P1, false},
		{"P4", P4, false},
		{" p2 ", P2, false},
		{"P0", "", true},
		{"P5", "", true},
		{"critical", "", true},
		{"", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseSeverity(tt.in)
			if tt.wantErr {
				if !errors.Is(err, apperr.ErrValidation) {
					t.Fatalf("want ErrValidation, got %v", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got (%q, %v), want %q", got, err, tt.want)
			}
		})
	}
}

func TestDeadlinesFrom(t *testing.T) {
	shanghai := time.FixedZone("CST", 8*3600)
	tests := []struct {
		name         string
		policy       Policy
		createdAt    time.Time
		wantResponse time.Time
		wantResolve  time.Time
	}{
		{
			name:         "一级 P1：15 分钟响应 / 2 小时恢复",
			policy:       Policy{Response: 15 * time.Minute, Resolve: 2 * time.Hour},
			createdAt:    time.Date(2026, 10, 30, 6, 0, 0, 0, time.UTC),
			wantResponse: time.Date(2026, 10, 30, 6, 15, 0, 0, time.UTC),
			wantResolve:  time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC),
		},
		{
			name:         "跨天跨月：三级 P3 恢复 72 小时",
			policy:       Policy{Response: 4 * time.Hour, Resolve: 72 * time.Hour},
			createdAt:    time.Date(2026, 10, 30, 22, 30, 0, 0, time.UTC),
			wantResponse: time.Date(2026, 10, 31, 2, 30, 0, 0, time.UTC),
			wantResolve:  time.Date(2026, 11, 2, 22, 30, 0, 0, time.UTC),
		},
		{
			name:         "北京时间输入，结果统一为 UTC 且是同一时刻",
			policy:       Policy{Response: 30 * time.Minute, Resolve: 4 * time.Hour},
			createdAt:    time.Date(2026, 10, 30, 14, 0, 0, 0, shanghai), // = 06:00 UTC
			wantResponse: time.Date(2026, 10, 30, 6, 30, 0, 0, time.UTC),
			wantResolve:  time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := tt.policy.DeadlinesFrom(tt.createdAt)
			if !d.Response.Equal(tt.wantResponse) || !d.Resolve.Equal(tt.wantResolve) {
				t.Errorf("got (%v, %v), want (%v, %v)", d.Response, d.Resolve, tt.wantResponse, tt.wantResolve)
			}
			if d.Response.Location() != time.UTC || d.Resolve.Location() != time.UTC {
				t.Errorf("deadlines should be UTC, got %v / %v", d.Response.Location(), d.Resolve.Location())
			}
		})
	}
}
