package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestStartExitsWhenRunReturns(t *testing.T) {
	code := make(chan int, 1)
	exit = func(c int) { code <- c; select {} }
	defer func() { exit = os.Exit }()
	p := &program{run: func(context.Context) error { return errors.New("database not reachable") }, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := p.Start(nil); err != nil {
		t.Fatal(err)
	}
	select {
	case c := <-code:
		if c != 1 {
			t.Fatalf("exit code %d, want 1", c)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("process kept running after the server returned")
	}
}

func TestStopDoesNotExit(t *testing.T) {
	exit = func(c int) { t.Errorf("exit(%d) after Stop", c) }
	defer func() { exit = os.Exit }()
	p := &program{run: func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }, log: slog.New(slog.NewTextHandler(io.Discard, nil))}
	if err := p.Start(nil); err != nil {
		t.Fatal(err)
	}
	if err := p.Stop(nil); err != nil {
		t.Fatal(err)
	}
}
