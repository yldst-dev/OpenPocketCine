package ffplay

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"nanomonitor/internal/domain"
)

func TestActualFFplay(t *testing.T) {
	if os.Getenv("NANO_MONITOR_REAL_PLAYER") != "1" {
		t.Skip("set NANO_MONITOR_REAL_PLAYER=1 to open a synthetic video window")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	data, err := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-f", "lavfi", "-i",
		"testsrc2=size=640x360:rate=25", "-t", "10", "-an", "-c:v", "libx264",
		"-preset", "ultrafast", "-tune", "zerolatency", "-f", "h264", "pipe:1").Output()
	if err != nil {
		t.Fatal(err)
	}
	player, err := New("ffplay")
	if err != nil {
		t.Fatal(err)
	}
	frames := make(chan domain.AccessUnit, 1)
	frames <- domain.AccessUnit{Data: data}
	close(frames)
	if err := player.Play(ctx, frames); err != nil {
		t.Fatal(err)
	}
}
