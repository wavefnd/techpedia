package auth

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPasswordWorkHasNoUnboundedWaitingQueueAndHonorsCancellation(t *testing.T) {
	var budget passwordWorkBudget
	ctx := context.Background()
	a, err := budget.acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, err := budget.acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := budget.acquire(ctx); !errors.Is(err, ErrBusy) {
		t.Fatal("concurrent work not rejected", err)
	}
	a()
	b()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := budget.acquire(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if budget.active != 0 {
		t.Fatal("admission slot leaked")
	}
}

func TestPasswordWorkRateBudgetRefillsWithoutWeakeningKDF(t *testing.T) {
	var budget passwordWorkBudget
	for range passwordBurst {
		release, err := budget.acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		release()
	}
	if _, err := budget.acquire(context.Background()); !errors.Is(err, ErrBusy) {
		t.Fatal("rate budget not enforced", err)
	}
	budget.updated = time.Now().Add(-2 * time.Second)
	release, err := budget.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	release()
}

func TestRegisterAndLoginSharePasswordAdmissionAndDuplicatesAvoidKDF(t *testing.T) {
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	_, err = s.Register(ctx, Registration{Username: "existing", Password: "existing-password"})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.passwordWork.acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.passwordWork.acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Register(ctx, Registration{Username: "otheruser", Password: "other-password"}); !errors.Is(err, ErrBusy) {
		t.Fatal("signup bypassed admission", err)
	}
	if _, _, _, err := s.Login(ctx, "existing", "wrong-password"); !errors.Is(err, ErrBusy) {
		t.Fatal("login bypassed admission", err)
	}
	if _, err := s.Register(ctx, Registration{Username: "existing", Password: "existing-password"}); !errors.Is(err, ErrUsernameTaken) {
		t.Fatal("duplicate signup performed KDF", err)
	}
	a()
	b()
	if _, token, _, err := s.Login(ctx, "existing", "existing-password"); err != nil || token == "" {
		t.Fatal("legitimate login failed", err)
	}
}
