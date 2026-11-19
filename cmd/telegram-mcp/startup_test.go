package main

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/imtaqin/telegram-cli/internal/telegram"
)

func TestWaitForClientReady(t *testing.T) {
	t.Run("ready", func(t *testing.T) {
		ready := make(chan struct{})
		close(ready)
		if err := waitForClientReady(ready, nil, nil); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("authorization required", func(t *testing.T) {
		errs := make(chan error, 1)
		want := fmt.Errorf("authorization: %w", telegram.ErrLoginRequired)
		errs <- want
		got := waitForClientReady(nil, errs, nil)
		if got != want || !errors.Is(got, telegram.ErrLoginRequired) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
	t.Run("other client error", func(t *testing.T) {
		errs := make(chan error, 1)
		want := errors.New("connection failed")
		errs <- want
		if got := waitForClientReady(nil, errs, nil); got != want {
			t.Fatalf("got %v, want %v", got, want)
		}
	})
	t.Run("timeout is not an authorization failure", func(t *testing.T) {
		timeout := make(chan time.Time, 1)
		timeout <- time.Time{}
		err := waitForClientReady(nil, nil, timeout)
		if err == nil {
			t.Fatal("expected timeout error")
		}
		if errors.Is(err, telegram.ErrLoginRequired) || strings.Contains(err.Error(), "login") || strings.Contains(err.Error(), "not authorized") {
			t.Fatalf("timeout incorrectly suggests authentication failure: %v", err)
		}
		if !strings.Contains(err.Error(), "timed out") || !strings.Contains(err.Error(), "network") {
			t.Fatalf("missing actionable timeout diagnosis: %v", err)
		}
	})
}
