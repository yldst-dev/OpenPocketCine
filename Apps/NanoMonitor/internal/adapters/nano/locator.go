package nano

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"nanomonitor/internal/domain"
)

type Locator struct {
	LocalAddress netip.Addr
	Candidates   func(context.Context) ([]netip.Addr, error)
	Report       func(string)
}

func (l Locator) Locate(ctx context.Context, identity domain.CameraIdentity) (netip.Addr, error) {
	if l.Report != nil {
		l.Report("공유기에서 같은 Nano의 주소를 확인합니다.")
	}
	var lastError error
	for attempt := 0; attempt < 2; attempt++ {
		candidates, err := l.Candidates(ctx)
		if err != nil {
			return netip.Addr{}, err
		}
		if len(candidates) > 8 {
			return netip.Addr{}, errors.New("응답한 후보가 8개를 넘었습니다. 공유기에서 Nano의 주소를 확인하고 setup -camera로 지정해 주세요")
		}
		for _, address := range candidates {
			checkCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
			camera := Camera{Address: address, LocalAddress: l.LocalAddress, ExpectedName: identity.Name}
			err := camera.Verify(checkCtx)
			cancel()
			if err == nil {
				return address, nil
			}
			lastError = err
			if ctx.Err() != nil {
				return netip.Addr{}, ctx.Err()
			}
		}
		if attempt == 0 {
			timer := time.NewTimer(3 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return netip.Addr{}, ctx.Err()
			case <-timer.C:
			}
		}
	}
	if lastError != nil {
		return netip.Addr{}, fmt.Errorf("Nano의 로컬 연결 확인에 실패했습니다: %w", lastError)
	}
	return netip.Addr{}, errors.New("같은 네트워크에서 설정한 Nano를 확인하지 못했습니다. 네트워크 격리와 로컬 네트워크 권한을 확인해 주세요")
}
