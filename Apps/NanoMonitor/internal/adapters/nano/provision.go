package nano

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"nanomonitor/internal/domain"
)

type BluetoothLink interface {
	Connect(context.Context, domain.Device) error
	Write(context.Context, []byte) error
	Read(context.Context) (byte, []byte, error)
	Close()
}

type Provisioner struct {
	Link   BluetoothLink
	Report func(string)
	timing provisionTiming
}

type provisionTiming struct {
	pace, wake, settle, retry, rolePoll time.Duration
	reply, pair, join                   time.Duration
}

func defaultProvisionTiming() provisionTiming {
	return provisionTiming{pace: 120 * time.Millisecond, wake: time.Second, settle: 10 * time.Second,
		retry: 5 * time.Second, rolePoll: 2 * time.Second,
		reply: 12 * time.Second, pair: 90 * time.Second, join: 45 * time.Second}
}

type bleSession struct {
	link          BluetoothLink
	sequence      uint16
	lastWrite     time.Time
	lastKeepalive time.Time
	approved      bool
	pairing       bool
	assemblers    map[byte]*notificationAssembler
	timing        provisionTiming
}

func (p Provisioner) open(ctx context.Context, device domain.Device) (*bleSession, error) {
	if p.timing.reply == 0 {
		p.timing = defaultProvisionTiming()
	}
	if p.Report == nil {
		p.Report = func(string) {}
	}
	p.Report("Nano에 Bluetooth로 연결합니다. 다른 카메라 앱은 종료해 주세요.")
	if err := p.Link.Connect(ctx, device); err != nil {
		return nil, err
	}
	var random [2]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, err
	}
	s := &bleSession{link: p.Link, sequence: le.Uint16(random[:]), timing: p.timing,
		assemblers: make(map[byte]*notificationAssembler), lastKeepalive: time.Now()}
	if err := s.send(ctx, s.stamp(command(0xf0, 0, 0x2b, []byte{4, 0}))); err != nil {
		return nil, err
	}
	p.Report("Nano 도크에 연결 요청이 나타나면 허용해 주세요.")
	s.pairing = true
	pairCtx, cancel := context.WithTimeout(ctx, s.timing.pair)
	defer cancel()
	request := s.stamp(pairing())
	if err := s.send(pairCtx, request); err != nil {
		return nil, fmt.Errorf("카메라 연결 승인 대기 실패: %w", err)
	}
	for !s.approved {
		frames, err := s.receive(pairCtx)
		if err != nil {
			return nil, err
		}
		for _, response := range frames {
			if response.flags&0x80 == 0 || response.seq != request.seq || response.set != 7 || response.id != 0x45 {
				continue
			}
			if bytes.Equal(response.payload, []byte{0, 1}) {
				s.approved = true
			} else if !bytes.Equal(response.payload, []byte{0, 2}) {
				return nil, errors.New("Nano가 Bluetooth 연결 승인을 거부했습니다")
			}
		}
	}
	s.pairing = false
	wake, err := s.exchange(ctx, command(0x1c, 0x53, 0x10, []byte{0, 0, 0, 0}), s.timing.reply)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(wake.payload, []byte{1, 0, 0, 0}) {
		return nil, errors.New("Nano가 Wi-Fi 준비를 확인하지 않았습니다")
	}
	if err := s.pause(ctx, s.timing.wake); err != nil {
		return nil, err
	}
	return s, nil
}

func (p Provisioner) Configure(ctx context.Context, device domain.Device, network domain.Network) (domain.CameraIdentity, error) {
	var identity domain.CameraIdentity
	if err := network.Validate(); err != nil {
		return identity, err
	}
	defer p.Link.Close()
	s, err := p.open(ctx, device)
	if err != nil {
		return identity, err
	}
	reply, err := s.exchange(ctx, command(7, 7, 7, nil), s.timing.reply)
	if err != nil {
		return identity, err
	}
	identity.Name, err = cameraName(reply.payload)
	if err != nil || !strings.HasPrefix(strings.ToLower(strings.ReplaceAll(identity.Name, " ", "")), "osmonano") {
		return domain.CameraIdentity{}, errors.New("Bluetooth로 연결한 기기가 Nano 형식의 이름을 반환하지 않았습니다")
	}
	if p.Report != nil {
		p.Report("Nano를 공유기 연결 모드로 전환합니다.")
	}
	role, err := s.exchange(ctx, command(7, 7, 0x39, []byte{0}), s.timing.reply)
	if err != nil {
		return identity, err
	}
	missing := bytes.Equal(role.payload, []byte{0xe0})
	if !bytes.Equal(role.payload, []byte{0, 1}) {
		if !missing && !bytes.Equal(role.payload, []byte{0, 0}) {
			return identity, errors.New("Nano가 지원하는 Wi-Fi 모드 응답이 아닙니다")
		}
		changed, err := s.exchange(ctx, command(7, 7, 0x48, []byte{1}), s.timing.reply)
		if err != nil {
			return identity, err
		}
		if !bytes.Equal(changed.payload, []byte{0, 0}) && !(missing && bytes.Equal(changed.payload, []byte{0})) {
			return identity, errors.New("Nano가 공유기 연결 모드 전환을 거부했습니다")
		}
		if !missing {
			ready := false
			for attempt := 0; attempt < 6; attempt++ {
				verified, err := s.exchange(ctx, command(7, 7, 0x39, []byte{0}), s.timing.reply)
				if err != nil {
					return identity, err
				}
				if bytes.Equal(verified.payload, []byte{0, 1}) {
					ready = true
					break
				}
				if !bytes.Equal(verified.payload, []byte{0, 0}) {
					return identity, errors.New("Wi-Fi 모드 전환 중 지원하지 않는 응답을 받았습니다")
				}
				if attempt < 5 {
					if err := s.pause(ctx, s.timing.rolePoll); err != nil {
						return identity, err
					}
				}
			}
			if !ready {
				return identity, errors.New("공유기 연결 모드 전환을 확인하지 못했습니다")
			}
		}
	}
	if err := s.pause(ctx, s.timing.settle); err != nil {
		return identity, err
	}
	payload := append([]byte{byte(len(network.SSID))}, network.SSID...)
	payload = append(payload, byte(len(network.Passphrase)))
	payload = append(payload, network.Passphrase...)
	defer clear(payload)
	for attempt := 1; attempt <= 3; attempt++ {
		if p.Report != nil {
			p.Report(fmt.Sprintf("Nano에 공유기 접속을 요청합니다(%d/3).", attempt))
		}
		joined, err := s.exchange(ctx, command(7, 7, 0x47, payload), s.timing.join)
		if err != nil {
			if ctx.Err() != nil {
				return identity, ctx.Err()
			}
			if errors.Is(err, context.DeadlineExceeded) {
				if p.Report != nil {
					p.Report("가입 응답을 받지 못했습니다. 실제 네트워크 연결 여부를 확인합니다.")
				}
				return identity, domain.ErrJoinUnconfirmed
			}
			return identity, err
		}
		if bytes.Equal(joined.payload, []byte{0, 0}) || bytes.Equal(joined.payload, []byte{0, 0, 0}) {
			if p.Report != nil {
				p.Report("Nano가 공유기 가입 요청을 받아들였습니다. 네트워크에서 연결을 확인합니다.")
			}
			return identity, nil
		}
		if !bytes.Equal(joined.payload, []byte{1, 0xff}) || attempt == 3 {
			return identity, errors.New("Nano가 공유기 접속을 거부했습니다. WPA2와 비밀번호를 확인해 주세요. 필요하면 setup -restore-ap로 카메라 Wi-Fi를 복원할 수 있습니다")
		}
		if err := s.pause(ctx, s.timing.retry); err != nil {
			return identity, err
		}
	}
	return identity, domain.ErrJoinUnconfirmed
}

func (p Provisioner) Restore(ctx context.Context, device domain.Device) error {
	defer p.Link.Close()
	s, err := p.open(ctx, device)
	if err != nil {
		return err
	}
	reply, err := s.exchange(ctx, command(7, 7, 0x48, []byte{0}), s.timing.reply)
	if err != nil {
		return err
	}
	if !bytes.Equal(reply.payload, []byte{0}) && !bytes.Equal(reply.payload, []byte{0, 0}) {
		return errors.New("카메라 Wi-Fi 복원 요청이 거부됐습니다")
	}
	return nil
}

func (s *bleSession) stamp(f frame) frame {
	s.sequence++
	f.seq = s.sequence
	return f
}

func (s *bleSession) send(ctx context.Context, f frame) error {
	if delay := s.timing.pace - time.Since(s.lastWrite); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	data := encodeFrame(f)
	defer clear(data)
	if err := s.link.Write(ctx, data); err != nil {
		return err
	}
	s.lastWrite = time.Now()
	return nil
}

func (s *bleSession) receive(ctx context.Context) ([]frame, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if time.Since(s.lastKeepalive) >= time.Second {
		if err := s.send(ctx, s.stamp(command(0xf0, 0, 0x2b, []byte{1, 1}))); err != nil {
			return nil, err
		}
		s.lastKeepalive = time.Now()
	}
	pollCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	channel, data, err := s.link.Read(pollCtx)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, nil
		}
		return nil, err
	}
	if channel != 4 && channel != 5 {
		return nil, nil
	}
	assembler := s.assemblers[channel]
	if assembler == nil {
		assembler = &notificationAssembler{}
		s.assemblers[channel] = assembler
	}
	frames := assembler.append(data)
	for _, f := range frames {
		if s.pairing && f.set == 7 && f.id == 0x46 && f.flags&0x80 == 0 {
			ack := command(7, 7, 0x46, []byte{0})
			ack.flags, ack.seq = 0xc0, f.seq
			if err := s.send(ctx, ack); err != nil {
				return nil, err
			}
			s.approved = true
		}
	}
	return frames, nil
}

func (s *bleSession) exchange(ctx context.Context, request frame, timeout time.Duration) (frame, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	request = s.stamp(request)
	if err := s.send(ctx, request); err != nil {
		return frame{}, err
	}
	for {
		frames, err := s.receive(ctx)
		if err != nil {
			return frame{}, err
		}
		for _, f := range frames {
			if f.flags&0x80 != 0 && f.seq == request.seq && f.set == request.set && f.id == request.id {
				return f, nil
			}
		}
	}
}

func (s *bleSession) pause(ctx context.Context, duration time.Duration) error {
	deadline := time.Now().Add(duration)
	for time.Now().Before(deadline) {
		if _, err := s.receive(ctx); err != nil {
			return err
		}
	}
	return ctx.Err()
}

type notificationAssembler struct {
	pending []byte
}

func (a *notificationAssembler) append(data []byte) []frame {
	if len(data) > 4096 {
		a.pending = nil
		return nil
	}
	if len(a.pending)+len(data) > 4096 {
		a.pending = nil
	}
	a.pending = append(a.pending, data...)
	var output []frame
	for len(a.pending) >= 4 {
		p := a.pending
		length := int(p[1]) | int(p[2]&3)<<8
		if p[0] != 0x55 || p[2]>>2 != 1 || crc8(p[:3]) != p[3] || length < 13 {
			a.pending = p[1:]
			continue
		}
		if len(p) < length {
			break
		}
		if crc16(p[:length-2]) == le.Uint16(p[length-2:length]) {
			frames := scanFrames(p[:length])
			if len(frames) == 1 {
				frames[0].payload = bytes.Clone(frames[0].payload)
				output = append(output, frames[0])
			}
			a.pending = p[length:]
		} else {
			a.pending = p[1:]
		}
	}
	return output
}
