//go:build !darwin || !cgo

package bluetooth

import (
	"context"
	"errors"

	"nanomonitor/internal/domain"
)

type Client struct{}

var unsupported = errors.New("Bluetooth 초기 설정은 현재 macOS의 CGO 빌드에서 지원합니다. 영상 모니터는 기존 플랫폼에서 그대로 사용할 수 있습니다")

func New() (*Client, error)                                       { return nil, unsupported }
func (*Client) Discover(context.Context) ([]domain.Device, error) { return nil, unsupported }
func (*Client) Connect(context.Context, domain.Device) error      { return unsupported }
func (*Client) Write(context.Context, []byte) error               { return unsupported }
func (*Client) Read(context.Context) (byte, []byte, error)        { return 0, nil, unsupported }
func (*Client) Close()                                            {}
