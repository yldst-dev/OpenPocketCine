package ffplay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"nanomonitor/internal/domain"
)

func TestMain(m *testing.M) {
	switch os.Getenv("NANO_MONITOR_TEST_PLAYER") {
	case "drain":
		io.Copy(io.Discard, os.Stdin)
		os.Exit(0)
	case "close":
		os.Exit(0)
	case "fail":
		fmt.Fprint(os.Stderr, "decoder failed")
		os.Exit(7)
	}
	os.Exit(m.Run())
}

func testPlayer(t *testing.T, mode string) *Player {
	t.Helper()
	t.Setenv("NANO_MONITOR_TEST_PLAYER", mode)
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	player, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	return player
}

func TestNormalWindowCloseIsNotBrokenPipeFailure(t *testing.T) {
	player := testPlayer(t, "close")
	frames := make(chan domain.AccessUnit, 1)
	frames <- domain.AccessUnit{Data: make([]byte, 1024*1024)}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := player.Play(ctx, frames); err != nil {
		t.Fatalf("normal player exit was an error: %v", err)
	}
}

func TestCancellationClosesBlockedPlayer(t *testing.T) {
	player := testPlayer(t, "drain")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := player.Play(ctx, make(chan domain.AccessUnit)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestPlayerFailureIncludesBoundedDiagnostic(t *testing.T) {
	player := testPlayer(t, "fail")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := player.Play(ctx, make(chan domain.AccessUnit))
	if err == nil || !strings.Contains(err.Error(), "decoder failed") {
		t.Fatalf("missing player diagnostic: %v", err)
	}
	var log limitedLog
	log.Write(make([]byte, 10000))
	if len(log.String()) != 4096 {
		t.Fatal("unbounded stderr")
	}
}
