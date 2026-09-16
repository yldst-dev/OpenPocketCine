package nano

import (
	"context"
	"net/netip"
	"strings"
	"testing"

	"nanomonitor/internal/domain"
)

func TestLocatorReportsTooManyCandidatesInsteadOfSilentlySkipping(t *testing.T) {
	locator := Locator{Candidates: func(context.Context) ([]netip.Addr, error) {
		return make([]netip.Addr, 9), nil
	}}
	_, err := locator.Locate(context.Background(), domain.CameraIdentity{Name: "OsmoNano-TEST"})
	if err == nil || !strings.Contains(err.Error(), "setup -camera") {
		t.Fatalf("candidate truncation was hidden: %v", err)
	}
}
