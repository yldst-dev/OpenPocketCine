//go:build darwin && cgo

package bluetooth

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fblocks
#cgo LDFLAGS: -framework Foundation -framework CoreBluetooth -Wl,-sectcreate,__TEXT,__info_plist,${SRCDIR}/Info.plist
#include "bridge.h"
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"
	"unsafe"

	"nanomonitor/internal/domain"
)

type Client struct {
	mu     sync.Mutex
	handle unsafe.Pointer
}

func New() (*Client, error) {
	handle := C.nano_ble_create()
	if handle == nil {
		return nil, errors.New("Bluetooth 권한 설명을 찾지 못했습니다. macOS에서 CGO_ENABLED=1로 다시 빌드해 주세요")
	}
	return &Client{handle: handle}, nil
}

func (c *Client) call(fn func(unsafe.Pointer) int) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.handle == nil {
		return 0, errors.New("Bluetooth 연결이 닫혔습니다")
	}
	value := fn(c.handle)
	if value < 0 {
		return value, fmt.Errorf("Bluetooth 작업을 완료하지 못했습니다(%d). 다른 카메라 앱을 닫고 Nano의 전원을 확인해 주세요", value)
	}
	return value, nil
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) Discover(ctx context.Context) ([]domain.Device, error) {
	powerCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		state, err := c.call(func(h unsafe.Pointer) int { return int(C.nano_ble_power(h)) })
		if err != nil {
			return nil, err
		}
		switch state {
		case 5:
			goto scan
		case 2:
			return nil, errors.New("이 컴퓨터에서 Bluetooth를 사용할 수 없습니다")
		case 3:
			return nil, errors.New("시스템 설정의 개인정보 보호 및 보안에서 실행 앱의 Bluetooth 접근을 허용해 주세요")
		case 4:
			return nil, errors.New("Mac의 Bluetooth를 켜 주세요")
		}
		if err := wait(powerCtx, 100*time.Millisecond); err != nil {
			return nil, err
		}
	}
scan:
	if _, err := c.call(func(h unsafe.Pointer) int { return int(C.nano_ble_scan(h)) }); err != nil {
		return nil, err
	}
	if err := wait(ctx, 6*time.Second); err != nil {
		return nil, err
	}
	buffer := make([]byte, 65536)
	n, err := c.call(func(h unsafe.Pointer) int {
		return int(C.nano_ble_devices(h, (*C.char)(unsafe.Pointer(&buffer[0])), C.size_t(len(buffer))))
	})
	if err != nil {
		return nil, err
	}
	var devices []domain.Device
	if err := json.Unmarshal(buffer[:n], &devices); err != nil {
		return nil, errors.New("Bluetooth 검색 결과를 읽지 못했습니다")
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].ID < devices[j].ID })
	return devices, nil
}

func (c *Client) Connect(ctx context.Context, device domain.Device) error {
	identifier := append([]byte(device.ID), 0)
	if len(device.ID) != 36 {
		return errors.New("Bluetooth 기기 ID가 올바르지 않습니다")
	}
	if _, err := c.call(func(h unsafe.Pointer) int {
		return int(C.nano_ble_connect(h, (*C.char)(unsafe.Pointer(&identifier[0]))))
	}); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		status, err := c.call(func(h unsafe.Pointer) int { return int(C.nano_ble_status(h)) })
		if err != nil || status == 1 {
			return err
		}
		if err := wait(ctx, 50*time.Millisecond); err != nil {
			return err
		}
	}
}

func (c *Client) Write(ctx context.Context, data []byte) error {
	if len(data) == 0 || len(data) > 1023 {
		return errors.New("Bluetooth 명령 길이가 올바르지 않습니다")
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		status, err := c.call(func(h unsafe.Pointer) int {
			return int(C.nano_ble_write(h, (*C.uint8_t)(unsafe.Pointer(&data[0])), C.size_t(len(data))))
		})
		if err != nil || status == 0 {
			return err
		}
		if err := wait(ctx, 20*time.Millisecond); err != nil {
			return err
		}
	}
}

func (c *Client) Read(ctx context.Context) (byte, []byte, error) {
	buffer := make([]byte, 4096)
	for {
		if err := ctx.Err(); err != nil {
			return 0, nil, err
		}
		var channel C.int
		n, err := c.call(func(h unsafe.Pointer) int {
			return int(C.nano_ble_read(h, (*C.uint8_t)(unsafe.Pointer(&buffer[0])), C.size_t(len(buffer)), &channel))
		})
		if err != nil {
			return 0, nil, err
		}
		if n > 0 {
			return byte(channel), buffer[:n], nil
		}
		if err := wait(ctx, 20*time.Millisecond); err != nil {
			return 0, nil, err
		}
	}
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.handle != nil {
		C.nano_ble_close(c.handle)
		c.handle = nil
	}
}
