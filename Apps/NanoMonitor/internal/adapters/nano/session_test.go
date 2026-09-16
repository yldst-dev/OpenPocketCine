package nano

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"nanomonitor/internal/domain"
)

type testCamera struct {
	udp        *net.UDPConn
	tcp        net.Listener
	mu         sync.Mutex
	commands   []frame
	acks       int
	packets    int
	gateWait   <-chan struct{}
	gateStatus byte
	video      []byte
}

func serveCamera(t *testing.T, name string, video []byte, configure ...func(*testCamera)) *testCamera {
	t.Helper()
	udp, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	tcp, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		udp.Close()
		t.Fatal(err)
	}
	fake := &testCamera{udp: udp, tcp: tcp, video: video}
	for _, option := range configure {
		option(fake)
	}
	t.Cleanup(func() { udp.Close(); tcp.Close() })
	go func() {
		conn, err := tcp.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		buffer := make([]byte, 2048)
		for {
			if _, err := conn.Read(buffer); err != nil {
				return
			}
		}
	}()
	go func() {
		buffer := make([]byte, 65536)
		for {
			n, peer, err := udp.ReadFromUDP(buffer)
			if err != nil {
				return
			}
			fake.mu.Lock()
			fake.packets++
			fake.mu.Unlock()
			p := buffer[:n]
			if !validPacket(p) {
				continue
			}
			sid := le.Uint16(p[2:4])
			switch p[6] {
			case 0:
				udp.WriteToUDP(packet(0, sid^0x80, 123, nil), peer)
				windows := make([]byte, 26)
				le.PutUint16(windows, 0x1238)
				le.PutUint16(windows[2:], 0x3000)
				le.PutUint16(windows[10:], 0x4000)
				le.PutUint16(windows[18:], 0x5000)
				udp.WriteToUDP(packet(1, sid, 0, windows), peer)
			case 4:
				fake.mu.Lock()
				fake.acks++
				fake.mu.Unlock()
			case 5:
				for _, f := range scanFrames(p[20:]) {
					f.payload = append([]byte(nil), f.payload...)
					fake.mu.Lock()
					fake.commands = append(fake.commands, f)
					fake.mu.Unlock()
					reply := frame{sender: f.receiver, receiver: 2, flags: 0xc0, set: f.set, id: f.id, seq: f.seq, payload: []byte{0}}
					if f.set == 2 && f.id == 9 && f.payload[10] == 3 {
						reply.payload = []byte{fake.gateStatus}
						if fake.gateWait != nil {
							go func(reply frame, sid uint16, peer *net.UDPAddr) {
								<-fake.gateWait
								udp.WriteToUDP(packet(3, sid, 0, encodeFrame(reply)), peer)
							}(reply, sid, peer)
							continue
						}
					}
					if f.set == 7 && f.id == 7 {
						reply.payload = append([]byte{0, byte(len(name))}, name...)
					}
					udp.WriteToUDP(packet(3, sid, 0, encodeFrame(reply)), peer)
					if f.set == 9 && f.id == 0xa8 {
						for _, fragment := range videoPackets(fake.video, 0xfff0, 1000) {
							udp.WriteToUDP(fragment, peer)
						}
					}
				}
			}
		}
	}()
	return fake
}

func (f *testCamera) adapter() Camera {
	return Camera{Address: netip.MustParseAddr("127.0.0.1"), LocalAddress: netip.MustParseAddr("127.0.0.1"),
		udpPort: f.udp.LocalAddr().(*net.UDPAddr).Port, tcpPort: f.tcp.Addr().(*net.TCPAddr).Port}
}

func TestSessionNegotiatesNanoAndOnlySendsPreviewCommands(t *testing.T) {
	data := fromHex(t, "000000016764001facb402802dd3501040106d0a13500000000168ee06f2c00000000165888421")
	fake := serveCamera(t, "OsmoNano-TEST", data)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	frames := make(chan domain.AccessUnit, 8)
	done := make(chan error, 1)
	go func() { done <- fake.adapter().Stream(ctx, frames) }()
	select {
	case unit := <-frames:
		if len(unit.Data) == 0 {
			t.Fatal("empty picture")
		}
	case err := <-done:
		t.Fatalf("stream ended before picture: %v", err)
	case <-ctx.Done():
		t.Fatal("no picture")
	}
	time.Sleep(80 * time.Millisecond)
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	allowed := map[uint16]bool{0x0707: true, 0x0081: true, 0x0088: true, 0x0099: true, 0x0209: true, 0x09a8: true}
	enables := 0
	for _, f := range fake.commands {
		if !allowed[uint16(f.set)<<8|uint16(f.id)] {
			t.Fatalf("non-preview command: %02x/%02x", f.set, f.id)
		}
		if f.set == 9 && f.id == 0xa8 {
			enables++
			if f.receiver != 0x41 {
				t.Fatal("incorrect Nano receiver")
			}
		}
	}
	if enables != 1 || fake.acks < 2 || len(fake.commands) == 0 || fake.commands[0].id != 7 {
		t.Fatalf("invalid session sequence: enables=%d ACKs=%d", enables, fake.acks)
	}
}

func TestSessionRejectsAnotherCameraBeforeRegistration(t *testing.T) {
	fake := serveCamera(t, "OsmoAction-TEST", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	camera := fake.adapter()
	camera.ExpectedName = "OsmoAction-TEST"
	err := camera.Stream(ctx, make(chan domain.AccessUnit, 8))
	if err == nil || !strings.Contains(err.Error(), "기기 이름이 아닙니다") {
		t.Fatalf("wrong identity result: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, f := range fake.commands {
		if f.set != 7 || f.id != 7 {
			t.Fatal("sent registration or preview before identity verification")
		}
	}
}

func TestSessionCancellationDuringStartup(t *testing.T) {
	fake := serveCamera(t, "OsmoNano-TEST", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := fake.adapter().Stream(ctx, make(chan domain.AccessUnit, 8))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("startup ignored cancellation: %v", err)
	}
}

func TestVerifyDoesNotStartPreview(t *testing.T) {
	fake := serveCamera(t, "OsmoNano-TEST", nil)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	camera := fake.adapter()
	camera.ExpectedName = "OsmoNano-TEST"
	if err := camera.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, f := range fake.commands {
		if f.set != 7 || f.id != 7 {
			t.Fatal("identity check changed camera preview")
		}
	}
}

func TestSessionReturnsTCPFailureBeforeUDP(t *testing.T) {
	fake := serveCamera(t, "OsmoNano-TEST", nil)
	camera := fake.adapter()
	fake.tcp.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := camera.Verify(ctx)
	if err == nil || !strings.Contains(err.Error(), "TCP 초기 연결 실패") {
		t.Fatalf("expected initial TCP failure, got %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.packets != 0 {
		t.Fatal("UDP session started after TCP failure")
	}
}

func TestPreviewWaitsForGateAcknowledgement(t *testing.T) {
	ready := make(chan struct{})
	fake := serveCamera(t, "OsmoNano-TEST", nil, func(f *testCamera) { f.gateWait = ready })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- fake.adapter().Stream(ctx, make(chan domain.AccessUnit, 8)) }()
	deadline := time.Now().Add(2 * time.Second)
	var found bool
	for time.Now().Before(deadline) {
		fake.mu.Lock()
		for _, command := range fake.commands {
			if command.set == 9 && command.id == 0xa8 {
				t.Error("enable preceded gate acknowledgement")
			}
			if command.set == 2 && command.id == 9 {
				found = true
			}
		}
		fake.mu.Unlock()
		if found {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	time.Sleep(50 * time.Millisecond)
	fake.mu.Lock()
	for _, command := range fake.commands {
		if command.set == 9 && command.id == 0xa8 {
			t.Error("enable preceded gate acknowledgement")
		}
	}
	fake.mu.Unlock()
	close(ready)
	if !found {
		t.Error("gate request missing")
	}
	cancel()
	<-done
}

func TestPreviewStopsOnGateRejection(t *testing.T) {
	fake := serveCamera(t, "OsmoNano-TEST", nil, func(f *testCamera) { f.gateStatus = 0xe0 })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err := fake.adapter().Stream(ctx, make(chan domain.AccessUnit, 8))
	if err == nil || !strings.Contains(err.Error(), "준비를 거부") {
		t.Fatalf("got %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, command := range fake.commands {
		if command.set == 9 && command.id == 0xa8 {
			t.Fatal("enable followed rejected gate")
		}
	}
}
