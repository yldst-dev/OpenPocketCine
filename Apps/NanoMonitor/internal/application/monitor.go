package application

import (
	"context"
	"errors"

	"nanomonitor/internal/domain"
)

type Camera interface {
	Stream(context.Context, chan<- domain.AccessUnit) error
}

type Display interface {
	Play(context.Context, <-chan domain.AccessUnit) error
}

func Monitor(ctx context.Context, camera Camera, display Display) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	frames := make(chan domain.AccessUnit, 8)
	sourceDone := make(chan error, 1)
	go func() {
		err := camera.Stream(runCtx, frames)
		sourceDone <- err
		close(frames)
		if err != nil {
			cancel()
		}
	}()
	displayErr := display.Play(runCtx, frames)
	cancel()
	sourceErr := <-sourceDone
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if sourceErr != nil && !errors.Is(sourceErr, context.Canceled) {
		return sourceErr
	}
	if errors.Is(displayErr, context.Canceled) {
		return nil
	}
	return displayErr
}
