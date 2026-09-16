package application

import (
	"context"
	"errors"
	"net/netip"
	"testing"

	"nanomonitor/internal/domain"
)

type provisionFunc func(context.Context, domain.Device, domain.Network) (domain.CameraIdentity, error)

func (f provisionFunc) Configure(ctx context.Context, device domain.Device, network domain.Network) (domain.CameraIdentity, error) {
	return f(ctx, device, network)
}

type locateFunc func(context.Context, domain.CameraIdentity) (netip.Addr, error)

func (f locateFunc) Locate(ctx context.Context, identity domain.CameraIdentity) (netip.Addr, error) {
	return f(ctx, identity)
}

func TestSetupRequiresLANVerificationEvenAfterJoinAck(t *testing.T) {
	for _, replyErr := range []error{nil, domain.ErrJoinUnconfirmed} {
		password := []byte("test-only-password")
		provisioner := provisionFunc(func(context.Context, domain.Device, domain.Network) (domain.CameraIdentity, error) {
			return domain.CameraIdentity{Name: "OsmoNano-TEST"}, replyErr
		})
		locator := locateFunc(func(_ context.Context, identity domain.CameraIdentity) (netip.Addr, error) {
			if identity.Name != "OsmoNano-TEST" {
				t.Fatal("lost paired identity")
			}
			for _, b := range password {
				if b != 0 {
					t.Fatal("password retained during network search")
				}
			}
			return netip.MustParseAddr("192.168.10.42"), nil
		})
		result, err := Setup(context.Background(), domain.Device{}, domain.Network{SSID: "TestNetwork", Passphrase: password}, provisioner, locator)
		if err != nil || result.Address.String() != "192.168.10.42" || result.Identity.Name != "OsmoNano-TEST" {
			t.Fatalf("verification failed: %v", err)
		}
	}
}

func TestSetupDoesNotClaimSuccessWhenLANCheckFails(t *testing.T) {
	provisioner := provisionFunc(func(context.Context, domain.Device, domain.Network) (domain.CameraIdentity, error) {
		return domain.CameraIdentity{Name: "OsmoNano-TEST"}, nil
	})
	missing := errors.New("not found")
	locator := locateFunc(func(context.Context, domain.CameraIdentity) (netip.Addr, error) {
		return netip.Addr{}, missing
	})
	result, err := Setup(context.Background(), domain.Device{}, domain.Network{SSID: "TestNetwork", Passphrase: []byte("test-only-password")}, provisioner, locator)
	if !errors.Is(err, missing) || result.Address.IsValid() {
		t.Fatal("join acknowledgement was treated as confirmed network access")
	}
}

func TestSetupValidatesBeforeBluetoothWrite(t *testing.T) {
	called := false
	provisioner := provisionFunc(func(context.Context, domain.Device, domain.Network) (domain.CameraIdentity, error) {
		called = true
		return domain.CameraIdentity{}, nil
	})
	_, err := Setup(context.Background(), domain.Device{}, domain.Network{}, provisioner, nil)
	if err == nil || called {
		t.Fatal("invalid credentials reached Bluetooth")
	}
}
