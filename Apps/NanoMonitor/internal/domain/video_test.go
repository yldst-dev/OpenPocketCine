package domain

import (
	"testing"
	"time"
)

func TestRecoveryDoesNotBecomePeriodicPLI(t *testing.T) {
	now := time.Unix(100, 0)
	if Recovery(now, now.Add(-time.Second), time.Time{}, true, 0) != KeepReceiving {
		t.Fatal("first picture grace was ignored")
	}
	if Recovery(now, now.Add(-time.Hour), now.Add(-time.Second), false, 2) != KeepReceiving {
		t.Fatal("old keyframe caused unnecessary enable")
	}
	if Recovery(now, now.Add(-9*time.Second), now.Add(-3*time.Second), false, 1) != RequestPicture {
		t.Fatal("stalled stream was not recovered")
	}
	if Recovery(now, now.Add(-9*time.Second), now.Add(-time.Second), true, 1) != RequestPicture {
		t.Fatal("lost reference picture was ignored")
	}
	if Recovery(now, now.Add(-9*time.Second), time.Time{}, true, 2) != StopStream {
		t.Fatal("unbounded recovery")
	}
}
