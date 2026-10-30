package alert

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/fh/opshub/backend/internal/apperr"
	"github.com/fh/opshub/backend/internal/sla"
)

func TestParseAlertmanagerPayload(t *testing.T) {
	body, err := os.ReadFile("testdata/firing.json")
	if err != nil {
		t.Fatal(err)
	}
	p, err := Parse(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if p.Version != "4" || len(p.Alerts) != 2 {
		t.Fatalf("version=%q alerts=%d", p.Version, len(p.Alerts))
	}

	a := p.Alerts[0]
	if a.Status != StatusFiring || a.Name() != "GovAppHighErrorRate" || a.System() != "GOV-APPLY" || a.Fingerprint != "a1b2c3d4e5f60718" {
		t.Errorf("first alert = %+v", a)
	}
	if want := time.Date(2026, 10, 30, 7, 10, 15, 123e6, time.UTC); !a.StartsAt.Equal(want) {
		t.Errorf("startsAt = %v, want %v", a.StartsAt, want)
	}
	if a.Title() != "GOV-APPLY 接口错误率超过 5%" {
		t.Errorf("title = %q", a.Title())
	}
	if p.Alerts[1].Status != StatusResolved {
		t.Errorf("second alert status = %q", p.Alerts[1].Status)
	}
}

func TestParseRejectsBadPayload(t *testing.T) {
	tests := map[string]string{
		"not json":            `{"alerts": [`,
		"missing fingerprint": `{"alerts": [{"status": "firing", "labels": {}}]}`,
		"bad status":          `{"alerts": [{"status": "pending", "fingerprint": "x"}]}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse([]byte(body)); !errors.Is(err, apperr.ErrValidation) {
				t.Errorf("want ErrValidation, got %v", err)
			}
		})
	}
}

func TestSeverity(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		want     sla.Severity
		wantNote bool
	}{
		{"P1", map[string]string{"severity": "P1"}, sla.P1, false},
		{"missing", map[string]string{}, sla.P3, true},
		{"invalid", map[string]string{"severity": "critical"}, sla.P3, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sev, note := Alert{Labels: tt.labels}.Severity()
			if sev != tt.want || (note != "") != tt.wantNote {
				t.Errorf("got (%s, %q), want %s, note=%v", sev, note, tt.want, tt.wantNote)
			}
		})
	}
}

func TestTitleFallback(t *testing.T) {
	a := Alert{Labels: map[string]string{"alertname": "GovAppProbeDown", "system": "GOV-APPLY"}}
	if got := a.Title(); got != "GovAppProbeDown [GOV-APPLY]" {
		t.Errorf("title = %q", got)
	}
}

func TestDecide(t *testing.T) {
	tests := []struct {
		status  string
		hasOpen bool
		want    Action
	}{
		{StatusFiring, false, CreateTicket},
		{StatusFiring, true, AppendDuplicate},
		{StatusResolved, true, AppendRecovered},
		{StatusResolved, false, Ignore},
	}
	for _, tt := range tests {
		got := Decide(Alert{Status: tt.status}, tt.hasOpen)
		if got != tt.want {
			t.Errorf("Decide(%s, open=%v) = %s, want %s", tt.status, tt.hasOpen, got, tt.want)
		}
	}
}
