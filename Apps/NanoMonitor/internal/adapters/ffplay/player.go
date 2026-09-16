package ffplay

import (
	"context"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"nanomonitor/internal/domain"
)

type Player struct {
	path string
}

func New(path string) (*Player, error) {
	resolved, err := exec.LookPath(path)
	if err != nil {
		return nil, fmt.Errorf("FFplay를 찾지 못했습니다. 설치하거나 -ffplay 경로를 지정해 주세요: %w", err)
	}
	return &Player{path: resolved}, nil
}

func (p *Player) Play(ctx context.Context, frames <-chan domain.AccessUnit) error {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(runCtx, p.path,
		"-hide_banner", "-loglevel", "error", "-autoexit", "-an", "-sn",
		"-fflags", "+genpts+discardcorrupt", "-flags", "low_delay", "-framedrop", "-sync", "ext",
		"-probesize", "32768", "-analyzeduration", "0", "-window_title", "Osmo Nano Monitor",
		"-f", "h264", "-i", "pipe:0")
	cmd.WaitDelay = time.Second
	logs := &limitedLog{}
	cmd.Stderr = logs
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("재생 입력을 열지 못했습니다: %w", err)
	}
	if err := cmd.Start(); err != nil {
		stdin.Close()
		return fmt.Errorf("FFplay 실행에 실패했습니다: %w", err)
	}
	writerDone := make(chan error, 1)
	go func() {
		defer stdin.Close()
		for {
			select {
			case <-runCtx.Done():
				writerDone <- runCtx.Err()
				return
			case unit, ok := <-frames:
				if !ok {
					writerDone <- nil
					return
				}
				if len(unit.Data) == 0 || len(unit.Data) > domain.MaxAccessUnit {
					writerDone <- fmt.Errorf("영상 프레임 크기가 올바르지 않습니다")
					cancel()
					return
				}
				if _, err := stdin.Write(unit.Data); err != nil {
					writerDone <- err
					return
				}
			}
		}
	}()
	err = cmd.Wait()
	cancel()
	stdin.Close()
	<-writerDone
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("영상 재생이 종료됐습니다: %w (%s)", err, logs.String())
	}
	return nil
}

type limitedLog struct {
	mu   sync.Mutex
	data []byte
}

func (b *limitedLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(p) >= 4096 {
		b.data = append(b.data[:0], p[len(p)-4096:]...)
	} else {
		b.data = append(b.data, p...)
		if len(b.data) > 4096 {
			b.data = append([]byte(nil), b.data[len(b.data)-4096:]...)
		}
	}
	return len(p), nil
}

func (b *limitedLog) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.data)
}
