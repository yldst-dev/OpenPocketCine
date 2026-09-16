package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"nanomonitor/internal/domain"
)

type sourceFunc func(context.Context, chan<- domain.AccessUnit) error

func (f sourceFunc) Stream(ctx context.Context, frames chan<- domain.AccessUnit) error {
	return f(ctx, frames)
}

type displayFunc func(context.Context, <-chan domain.AccessUnit) error

func (f displayFunc) Play(ctx context.Context, frames <-chan domain.AccessUnit) error {
	return f(ctx, frames)
}

func TestDisplayCloseCancelsSource(t *testing.T) {
	stopped := make(chan struct{})
	camera := sourceFunc(func(ctx context.Context, out chan<- domain.AccessUnit) error {
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	})
	display := displayFunc(func(context.Context, <-chan domain.AccessUnit) error { return nil })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Monitor(ctx, camera, display); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("source outlived the monitor")
	}
}

func TestSourceErrorSurvivesDisplayCancellation(t *testing.T) {
	want := errors.New("source stopped")
	camera := sourceFunc(func(context.Context, chan<- domain.AccessUnit) error { return want })
	display := displayFunc(func(ctx context.Context, _ <-chan domain.AccessUnit) error {
		<-ctx.Done()
		return ctx.Err()
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Monitor(ctx, camera, display); !errors.Is(err, want) {
		t.Fatalf("lost source error: %v", err)
	}
}
