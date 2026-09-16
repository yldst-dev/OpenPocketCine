package application

import (
	"context"
	"errors"
	"fmt"
	"net/netip"

	"nanomonitor/internal/domain"
)

type Provisioner interface {
	Configure(context.Context, domain.Device, domain.Network) (domain.CameraIdentity, error)
}

type Locator interface {
	Locate(context.Context, domain.CameraIdentity) (netip.Addr, error)
}

type SetupResult struct {
	Address  netip.Addr
	Identity domain.CameraIdentity
}

func Setup(ctx context.Context, device domain.Device, network domain.Network, provisioner Provisioner, locator Locator) (SetupResult, error) {
	defer clear(network.Passphrase)
	if err := network.Validate(); err != nil {
		return SetupResult{}, err
	}
	identity, err := provisioner.Configure(ctx, device, network)
	clear(network.Passphrase)
	if err != nil && !errors.Is(err, domain.ErrJoinUnconfirmed) {
		return SetupResult{}, err
	}
	if ctx.Err() != nil {
		return SetupResult{}, ctx.Err()
	}
	if identity.Name == "" {
		return SetupResult{}, errors.New("설정한 카메라의 식별 정보가 없습니다")
	}
	address, locateErr := locator.Locate(ctx, identity)
	if locateErr != nil {
		return SetupResult{}, fmt.Errorf("공유기 접속 명령 후 같은 Nano의 네트워크 연결을 확인하지 못했습니다. 카메라 모드가 변경됐을 수 있습니다. 비밀번호와 WPA2 설정을 확인하거나 setup -restore-ap로 복원해 주세요: %w", locateErr)
	}
	return SetupResult{Address: address, Identity: identity}, nil
}
