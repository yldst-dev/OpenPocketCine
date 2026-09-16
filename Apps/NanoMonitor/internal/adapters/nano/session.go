package nano

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"strings"
	"time"

	"nanomonitor/internal/domain"
)

type Camera struct {
	Address      netip.Addr
	LocalAddress netip.Addr
	ExpectedName string
	Report       func(string)
	udpPort      int
	tcpPort      int
}

type session struct {
	conn              *net.UDPConn
	id, base, seq     uint16
	dumlSeq           uint16
	counter           byte
	windows           windows
	assembler         assembler
	gate              avcGate
	lastEnable        time.Time
	lastPicture       time.Time
	healthySince      time.Time
	recoveries        int
	lostPictures      uint64
	enableSequence    uint16
	identitySequence  uint16
	identityRequested bool
	registered        bool
	enabled           bool
	gateStarted       bool
	report            func(string)
}

var errPreviewStalled = errors.New("영상 복구 2회가 실패했습니다")

func (c Camera) Stream(ctx context.Context, frames chan<- domain.AccessUnit) error {
	return reconnectPreview(ctx, func() error { return c.run(ctx, frames, false) }, c.Report)
}

func reconnectPreview(ctx context.Context, run func() error, report func(string)) error {
	for attempt := 0; ; attempt++ {
		err := run()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !errors.Is(err, errPreviewStalled) {
			return err
		}
		if attempt == 2 {
			return fmt.Errorf("연결을 2회 새로 맺었지만 영상을 복구하지 못했습니다: %w", err)
		}
		if report != nil {
			report(fmt.Sprintf("영상 연결을 새로 맺습니다(%d/2). 영상 창은 유지합니다.", attempt+1))
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (c Camera) Verify(ctx context.Context) error {
	return c.run(ctx, nil, true)
}

func (c *Camera) run(ctx context.Context, frames chan<- domain.AccessUnit, identifyOnly bool) error {
	if !c.Address.Is4() || !c.LocalAddress.Is4() {
		return errors.New("카메라와 로컬 IPv4 주소가 필요합니다")
	}
	if c.udpPort == 0 {
		c.udpPort = 9004
	}
	if c.tcpPort == 0 {
		c.tcpPort = 7001
	}
	if c.Report == nil {
		c.Report = func(string) {}
	}
	localIP := net.IP(c.LocalAddress.AsSlice())
	dialer := net.Dialer{LocalAddr: &net.TCPAddr{IP: localIP}, Timeout: 2 * time.Second}
	tcp, tcpErr := dialer.DialContext(ctx, "tcp4", net.JoinHostPort(c.Address.String(), fmt.Sprint(c.tcpPort)))
	if tcpErr == nil {
		defer tcp.Close()
		tcp.SetWriteDeadline(time.Now().Add(time.Second))
		if _, err := tcp.Write(encodeFrame(pairing())); err != nil {
			return fmt.Errorf("카메라 연결 초기화 실패: %w", err)
		}
		go func() { _, _ = io.Copy(io.Discard, tcp) }()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(400 * time.Millisecond):
		}
	} else {
		return fmt.Errorf("카메라 TCP 초기 연결 실패: %w", tcpErr)
	}
	conn, err := net.DialUDP("udp4", &net.UDPAddr{IP: localIP}, &net.UDPAddr{IP: net.IP(c.Address.AsSlice()), Port: c.udpPort})
	if err != nil {
		return fmt.Errorf("영상 연결을 열지 못했습니다: %w", err)
	}
	defer conn.Close()
	if err := conn.SetReadBuffer(4 * 1024 * 1024); err != nil {
		return fmt.Errorf("영상 수신 버퍼 설정 실패: %w", err)
	}
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	s := session{conn: conn, id: le.Uint16(random[:2]) | 0x1000,
		base: le.Uint16(random[2:]) & 0xfff8, dumlSeq: 0xa000, report: c.Report}
	s.windows = windows{video: s.base, data: s.base, extra: s.base}
	readCtx, stopReader := context.WithCancel(ctx)
	defer stopReader()
	packets := make(chan []byte, 256)
	readErrors := make(chan error, 1)
	go func() {
		buffer := make([]byte, 65536)
		for {
			n, err := conn.Read(buffer)
			if err != nil {
				readErrors <- err
				return
			}
			if !validPacket(buffer[:n]) {
				continue
			}
			p := append([]byte(nil), buffer[:n]...)
			select {
			case packets <- p:
			case <-readCtx.Done():
				return
			default:
				readErrors <- errors.New("영상 패킷을 제때 처리하지 못했습니다")
				return
			}
		}
	}()
	defer func() {
		if s.gateStarted {
			_, _ = s.send(previewGate(false))
		}
	}()
	ackTick := time.NewTicker(25 * time.Millisecond)
	defer ackTick.Stop()
	presenceTick := time.NewTicker(time.Second)
	defer presenceTick.Stop()
	handshakeTick := time.NewTicker(800 * time.Millisecond)
	defer handshakeTick.Stop()
	watchdog := time.NewTicker(250 * time.Millisecond)
	defer watchdog.Stop()
	startup := time.NewTimer(15 * time.Second)
	defer startup.Stop()
	var handshaken, windowReady, statusSeen bool
	var enableAt, gateDeadline time.Time
	var gateSequence uint16
	sends := 0
	sendHandshake := func() error {
		sends++
		p := packet(0, s.id, s.seq, handshake(s.base))
		s.seq += 8
		return s.write(p)
	}
	if err := sendHandshake(); err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readErrors:
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("카메라 수신이 중단됐습니다: %w", err)
		case <-startup.C:
			if !s.enabled {
				return errors.New("카메라가 연결 절차에 응답하지 않았습니다. 공유기 연결, 카메라 IP, 클라이언트 격리 설정을 확인해 주세요")
			}
		case <-handshakeTick.C:
			if !handshaken && sends < 5 {
				if err := sendHandshake(); err != nil {
					return err
				}
			}
		case <-ackTick.C:
			if handshaken {
				if err := s.write(packet(4, s.id, 0, s.windows.payload())); err != nil {
					return err
				}
			}
		case <-presenceTick.C:
			if s.registered {
				if _, err := s.send(presence()); err != nil {
					return err
				}
			}
		case now := <-watchdog.C:
			if !s.enabled && !enableAt.IsZero() && !now.Before(enableAt) {
				seq, err := s.send(previewGate(true))
				if err != nil {
					return err
				}
				s.gateStarted = true
				gateSequence = seq
				gateDeadline = now.Add(3 * time.Second)
				enableAt = time.Time{}
				startup.Stop()
				c.Report("영상 수신을 기다립니다. 다른 모니터나 DJI Mimo의 연결은 종료해 주세요.")
			}
			if !gateDeadline.IsZero() && !now.Before(gateDeadline) {
				return errors.New("Nano의 미리보기 준비 응답을 받지 못했습니다")
			}
			if s.enabled && gateDeadline.IsZero() {
				switch domain.Recovery(now, s.lastEnable, s.lastPicture, !s.gate.ready, s.recoveries) {
				case domain.RequestPicture:
					s.recoveries++
					s.healthySince = time.Time{}
					c.Report(fmt.Sprintf("영상 복구를 요청합니다. 손실 감지=%d, 무시한 지연·중복 패킷=%d", s.lostPictures, s.assembler.latePackets))
					seq, err := s.send(previewGate(true))
					if err != nil {
						return err
					}
					s.gateStarted = true
					gateSequence = seq
					gateDeadline = now.Add(3 * time.Second)
				case domain.StopStream:
					return errPreviewStalled
				}
			}
		case p := <-packets:
			s.windows.observe(p)
			if p[6] == 0 {
				handshaken = true
			}
			if p[6] == 1 && len(p) == 34 && !windowReady {
				windowReady = true
				s.seq = le.Uint16(p[8:10]) + 8
			}
			if handshaken && windowReady && !s.identityRequested {
				seq, err := s.send(command(7, 7, 7, nil))
				if err != nil {
					return err
				}
				s.identitySequence, s.identityRequested = seq, true
			}
			if p[6] == 2 {
				if s.enabled {
					if err := s.video(p, frames); err != nil {
						return err
					}
				}
				continue
			}
			for _, f := range scanFrames(p[8:]) {
				if !statusSeen && f.set == 2 && f.id == 0x80 && len(f.payload) >= 13 {
					statusSeen = true
					c.Report(fmt.Sprintf("카메라 상태: playback=%t recording=%t", le.Uint32(f.payload)&0x40000000 != 0, le.Uint32(f.payload)&0x80 != 0))
				}
				if f.flags&0x80 == 0 {
					continue
				}
				if s.identityRequested && !s.registered && f.set == 7 && f.id == 7 && f.seq == s.identitySequence {
					name, err := cameraName(f.payload)
					if err != nil {
						return err
					}
					if !strings.HasPrefix(strings.ToLower(strings.ReplaceAll(name, " ", "")), "osmonano") {
						return errors.New("Osmo Nano 형식의 기기 이름이 아닙니다. 이 프로그램은 OsmoNano로 시작하는 기본 카메라 이름만 지원합니다")
					}
					if c.ExpectedName != "" {
						if name != c.ExpectedName {
							return errors.New("응답한 카메라가 -name으로 지정한 Nano와 다릅니다")
						}
					}
					c.ExpectedName = name
					if identifyOnly {
						return nil
					}
					for _, message := range []frame{deviceInfo(), presence(), subscription("cam_status", 0x69df), subscription("cam_video_param_v2", 0x69e0)} {
						if _, err := s.send(message); err != nil {
							return err
						}
					}
					s.registered = true
					enableAt = time.Now().Add(300 * time.Millisecond)
					c.Report("Nano 형식의 기기 이름을 확인했습니다. 미리보기를 시작합니다.")
				}
				if !gateDeadline.IsZero() && f.set == 2 && f.id == 9 && f.seq == gateSequence && len(f.payload) > 0 {
					if f.payload[0] != 0 {
						return fmt.Errorf("Nano가 미리보기 준비를 거부했습니다(0x%02x)", f.payload[0])
					}
					gateDeadline = time.Time{}
					if err := s.enable(time.Now()); err != nil {
						return err
					}
				}
				if s.enabled && f.set == 9 && f.id == 0xa8 && f.seq == s.enableSequence && len(f.payload) > 0 && f.payload[0] != 0 {
					return fmt.Errorf("Nano가 미리보기를 거부했습니다(0x%02x). 다른 카메라 앱 연결과 카메라 상태를 확인해 주세요", f.payload[0])
				}
			}
		}
	}
}

func (s *session) write(p []byte) error {
	if err := s.conn.SetWriteDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
		return err
	}
	_, err := s.conn.Write(p)
	return err
}

func (s *session) send(f frame) (uint16, error) {
	f.seq = s.dumlSeq
	s.dumlSeq++
	s.counter++
	b := make([]byte, 12)
	le.PutUint16(b, s.seq-8)
	le.PutUint16(b[2:4], s.seq)
	b[8], b[9] = s.counter, 1
	b = append(b, encodeFrame(f)...)
	p := packet(5, s.id, s.seq, b)
	s.seq += 8
	return f.seq, s.write(p)
}

func (s *session) enable(now time.Time) error {
	seq, err := s.send(enablePreview())
	if err != nil {
		return err
	}
	s.enabled, s.enableSequence, s.lastEnable = true, seq, now
	s.assembler.reset()
	s.gate.ready = false
	return nil
}

func (s *session) video(p []byte, output chan<- domain.AccessUnit) error {
	data, lost := s.assembler.feed(p)
	if lost {
		s.lostPictures++
		s.gate.ready = false
		s.healthySince = time.Time{}
	}
	if data == nil {
		return nil
	}
	clean, err := s.gate.accept(data)
	if err != nil {
		return err
	}
	if clean == nil {
		return nil
	}
	select {
	case output <- domain.AccessUnit{Data: clean}:
	default:
		return errors.New("재생기가 영상을 제때 처리하지 못했습니다. 다른 고부하 작업을 종료한 뒤 다시 실행해 주세요")
	}
	now := time.Now()
	if s.lastPicture.IsZero() {
		s.report("AVC 영상 데이터를 수신하고 있습니다. 영상 창에서 Esc를 누르면 종료합니다.")
	}
	s.lastPicture = now
	if s.healthySince.IsZero() {
		s.healthySince = now
	}
	if now.Sub(s.healthySince) >= 30*time.Second {
		s.recoveries = 0
	}
	return nil
}
