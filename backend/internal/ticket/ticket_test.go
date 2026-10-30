package ticket

import (
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/fh/opshub/backend/internal/apperr"
)

// TestTransitionMatrix 覆盖 5 个状态 × 4 个动作的全部 20 种组合。
// 期望表独立书写，不引用实现里的 transitions，否则实现写错测试也跟着错。
func TestTransitionMatrix(t *testing.T) {
	legal := map[string]Status{
		"new/ack":              Acknowledged,
		"acknowledged/resolve": Resolved,
		"reopened/resolve":     Resolved,
		"resolved/close":       Closed,
		"resolved/reopen":      Reopened,
	}

	var nLegal, nIllegal int
	for _, cur := range AllStatuses {
		for _, act := range AllActions {
			key := fmt.Sprintf("%s/%s", cur, act)
			t.Run(key, func(t *testing.T) {
				got, err := Transition(cur, act)
				if want, ok := legal[key]; ok {
					nLegal++
					if err != nil || got != want {
						t.Errorf("got (%q, %v), want %q", got, err, want)
					}
					return
				}
				nIllegal++
				if !errors.Is(err, apperr.ErrInvalidTransition) {
					t.Errorf("want ErrInvalidTransition, got (%q, %v)", got, err)
				}
			})
		}
	}
	if nLegal != 5 || nIllegal != 15 {
		t.Errorf("covered %d legal + %d illegal, want 5 + 15", nLegal, nIllegal)
	}
}

func TestApplyEffects(t *testing.T) {
	t1 := time.Date(2026, 10, 30, 7, 0, 0, 0, time.UTC)
	t2 := t1.Add(time.Hour)
	tests := []struct {
		name  string
		in    Snapshot
		act   Action
		check func(t *testing.T, out Snapshot)
	}{
		{"ack sets responded_at and assignee", Snapshot{Type: TypeIncident, Status: New}, Ack,
			func(t *testing.T, out Snapshot) {
				if out.RespondedAt == nil || !out.RespondedAt.Equal(t2) || out.Assignee != "张工" {
					t.Errorf("out = %+v", out)
				}
			}},
		{"resolve sets resolved_at", Snapshot{Type: TypeIncident, Status: Acknowledged, RespondedAt: &t1}, Resolve,
			func(t *testing.T, out Snapshot) {
				if out.ResolvedAt == nil || !out.ResolvedAt.Equal(t2) || !out.RespondedAt.Equal(t1) {
					t.Errorf("out = %+v", out)
				}
			}},
		{"close sets closed_at, keeps resolved_at", Snapshot{Type: TypeIncident, Status: Resolved, ResolvedAt: &t1}, Close,
			func(t *testing.T, out Snapshot) {
				if out.ClosedAt == nil || !out.ClosedAt.Equal(t2) || !out.ResolvedAt.Equal(t1) {
					t.Errorf("out = %+v", out)
				}
			}},
		{"reopen clears resolved_at", Snapshot{Type: TypeIncident, Status: Resolved, ResolvedAt: &t1}, Reopen,
			func(t *testing.T, out Snapshot) {
				if out.Status != Reopened || out.ResolvedAt != nil {
					t.Errorf("out = %+v", out)
				}
			}},
		{"resolve after reopen keeps original assignee", Snapshot{Type: TypeIncident, Status: Reopened, Assignee: "李工"}, Resolve,
			func(t *testing.T, out Snapshot) {
				if out.Assignee != "李工" {
					t.Errorf("assignee = %q, want 李工", out.Assignee)
				}
			}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := Apply(tt.in, tt.act, "张工", t2)
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, out)
		})
	}
}

func TestApplyRejects(t *testing.T) {
	now := time.Now()
	if _, err := Apply(Snapshot{Type: TypeProblem, Status: New}, Ack, "张工", now); !errors.Is(err, apperr.ErrInvalidTransition) {
		t.Errorf("problem ticket: want ErrInvalidTransition, got %v", err)
	}
	if _, err := Apply(Snapshot{Type: TypeIncident, Status: Closed}, Reopen, "张工", now); !errors.Is(err, apperr.ErrInvalidTransition) {
		t.Errorf("closed is terminal: want ErrInvalidTransition, got %v", err)
	}
}

// 走一遍完整路径：new → ack → resolve → reopen → resolve → close。
func TestFullLifecycle(t *testing.T) {
	s := Snapshot{Type: TypeIncident, Status: New}
	now := time.Date(2026, 10, 30, 7, 0, 0, 0, time.UTC)
	for _, act := range []Action{Ack, Resolve, Reopen, Resolve, Close} {
		now = now.Add(10 * time.Minute)
		var err error
		if s, err = Apply(s, act, "张工", now); err != nil {
			t.Fatalf("%s: %v", act, err)
		}
	}
	if s.Status != Closed || !s.ResolvedAt.Equal(now.Add(-10*time.Minute)) {
		t.Errorf("final = %+v（resolved_at 应为第二次恢复的时间）", s)
	}
}

func TestParseAction(t *testing.T) {
	if a, err := ParseAction("ack"); err != nil || a != Ack {
		t.Errorf("ack: got (%q, %v)", a, err)
	}
	if _, err := ParseAction("delete"); !errors.Is(err, apperr.ErrValidation) {
		t.Errorf("delete: want ErrValidation, got %v", err)
	}
}
