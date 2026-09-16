package nano

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"nanomonitor/internal/domain"
)

type bleNotice struct {
	channel byte
	data    []byte
}

type fakeBluetooth struct {
	notifications chan bleNotice
	writes        []frame
	joins         [][]byte
	joinCount     int
	role          []byte
	readbacks     [][]byte
	roleSet       bool
	roleReads     int
	approvalOnly  bool
	silentPair    bool
	closed        bool
}

func newFakeBluetooth() *fakeBluetooth {
	return &fakeBluetooth{notifications: make(chan bleNotice, 64), role: []byte{0xe0}, joins: [][]byte{{0, 0}}}
}

func (b *fakeBluetooth) Connect(context.Context, domain.Device) error { return nil }
func (b *fakeBluetooth) Close()                                       { b.closed = true }

func (b *fakeBluetooth) Write(ctx context.Context, data []byte) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	frames := scanFrames(data)
	if len(frames) != 1 {
		return errors.New("invalid wire data")
	}
	f := frames[0]
	f.payload = bytes.Clone(f.payload)
	b.writes = append(b.writes, f)
	if f.flags&0x80 != 0 || f.set == 0 {
		return nil
	}
	reply := frame{sender: f.receiver, receiver: f.sender, seq: f.seq, flags: 0xc0, set: f.set, id: f.id}
	switch uint16(f.set)<<8 | uint16(f.id) {
	case 0x0745:
		if b.silentPair {
			return nil
		}
		if b.approvalOnly {
			request := frame{sender: 7, receiver: 2, seq: 0x3333, flags: 0x40, set: 7, id: 0x46, payload: []byte{0}}
			b.enqueue(request)
			return nil
		}
		reply.payload = []byte{0, 1}
	case 0x5310:
		reply.payload = []byte{1, 0, 0, 0}
	case 0x0707:
		name := "OsmoNano-TEST"
		reply.payload = append([]byte{0, byte(len(name))}, name...)
	case 0x0739:
		b.roleReads++
		reply.payload = b.role
		if b.roleSet && len(b.readbacks) > 0 {
			reply.payload = b.readbacks[0]
			b.readbacks = b.readbacks[1:]
		}
	case 0x0748:
		reply.payload = []byte{0}
		b.roleSet = true
		if !bytes.Equal(b.role, []byte{0xe0}) {
			reply.payload = []byte{0, 0}
		}
	case 0x0747:
		b.joinCount++
		if b.joinCount > len(b.joins) {
			return nil
		}
		reply.payload = b.joins[b.joinCount-1]
	default:
		return errors.New("unexpected command")
	}
	b.enqueue(reply)
	return nil
}

func (b *fakeBluetooth) enqueue(f frame) {
	data := encodeFrame(f)
	b.notifications <- bleNotice{4, bytes.Clone(data[:5])}
	b.notifications <- bleNotice{4, bytes.Clone(data[5:])}
}

func (b *fakeBluetooth) Read(ctx context.Context) (byte, []byte, error) {
	select {
	case <-ctx.Done():
		return 0, nil, ctx.Err()
	case notification := <-b.notifications:
		return notification.channel, notification.data, nil
	}
}

func testProvisioner(link *fakeBluetooth) Provisioner {
	return Provisioner{Link: link, timing: provisionTiming{reply: 100 * time.Millisecond,
		pair: 100 * time.Millisecond, join: 50 * time.Millisecond}}
}

func TestProvisionUsesCapturedNanoCommandsAndNeverChangesShooting(t *testing.T) {
	link := newFakeBluetooth()
	link.approvalOnly = true
	p := testProvisioner(link)
	var messages []string
	p.Report = func(message string) { messages = append(messages, message) }
	identity, err := p.Configure(context.Background(), domain.Device{}, domain.Network{SSID: "TestNetwork", Passphrase: []byte("test-only-password")})
	if err != nil || identity.Name != "OsmoNano-TEST" || !link.closed {
		t.Fatalf("configuration failed: %v", err)
	}
	allowed := map[uint16]bool{0x002b: true, 0x0745: true, 0x0746: true, 0x5310: true, 0x0707: true, 0x0739: true, 0x0748: true, 0x0747: true}
	var acknowledged, configured bool
	for _, f := range link.writes {
		key := uint16(f.set)<<8 | uint16(f.id)
		if !allowed[key] {
			t.Fatalf("unexpected opcode: %x", key)
		}
		if key == 0x0746 {
			acknowledged = f.seq == 0x3333 && f.flags == 0xc0 && bytes.Equal(f.payload, []byte{0})
		}
		if key == 0x0747 {
			want := append([]byte{11}, []byte("TestNetwork")...)
			want = append(want, 18)
			want = append(want, []byte("test-only-password")...)
			configured = bytes.Equal(f.payload, want)
		}
	}
	if !acknowledged || !configured {
		t.Fatal("pairing approval or credential payload mismatch")
	}
	if strings.Contains(strings.Join(messages, " "), "test-only-password") {
		t.Fatal("credentials appeared in a progress message")
	}
}

func TestJoinRetriesOnlyKnownTransientReply(t *testing.T) {
	for _, test := range []struct {
		name    string
		replies [][]byte
		count   int
		success bool
	}{
		{"transient", [][]byte{{1, 0xff}, {0, 0}}, 2, true},
		{"rejected", [][]byte{{0xe0}}, 1, false},
		{"budget", [][]byte{{1, 0xff}, {1, 0xff}, {1, 0xff}}, 3, false},
		{"trailing", [][]byte{{0, 0, 0, 0}}, 1, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			link := newFakeBluetooth()
			link.joins = test.replies
			_, err := testProvisioner(link).Configure(context.Background(), domain.Device{}, domain.Network{SSID: "TestNetwork", Passphrase: []byte("test-only-password")})
			if (err == nil) != test.success || link.joinCount != test.count {
				t.Fatalf("retry policy: count=%d err=%v", link.joinCount, err)
			}
		})
	}
}

func TestJoinTimeoutPreservesIdentityForLANVerification(t *testing.T) {
	link := newFakeBluetooth()
	link.joins = nil
	identity, err := testProvisioner(link).Configure(context.Background(), domain.Device{}, domain.Network{SSID: "TestNetwork", Passphrase: []byte("test-only-password")})
	if !errors.Is(err, domain.ErrJoinUnconfirmed) || identity.Name != "OsmoNano-TEST" || link.joinCount != 1 {
		t.Fatalf("uncertain join was mishandled: %v", err)
	}
}

func TestStationReadbackWaitsForTransitionAndIsBounded(t *testing.T) {
	for _, success := range []bool{true, false} {
		link := newFakeBluetooth()
		link.role = []byte{0, 0}
		if success {
			link.readbacks = [][]byte{{0, 0}, {0, 1}}
		}
		_, err := testProvisioner(link).Configure(context.Background(), domain.Device{}, domain.Network{SSID: "TestNetwork", Passphrase: []byte("test-only-password")})
		if (err == nil) != success {
			t.Fatalf("asynchronous role transition: %v", err)
		}
		if success && link.roleReads != 3 {
			t.Fatal("did not retry transitional AP response")
		}
		if !success && (link.roleReads != 7 || link.joinCount != 0) {
			t.Fatal("readback budget exceeded or unverified role used")
		}
	}
}

func TestCancelledPairingDoesNotConfigureWiFi(t *testing.T) {
	link := newFakeBluetooth()
	link.silentPair = true
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := testProvisioner(link).Configure(ctx, domain.Device{}, domain.Network{SSID: "TestNetwork", Passphrase: []byte("test-only-password")})
	if !errors.Is(err, context.DeadlineExceeded) || !link.closed {
		t.Fatalf("cancellation did not close the session: %v", err)
	}
	for _, f := range link.writes {
		if f.set == 7 && (f.id == 0x48 || f.id == 0x47) {
			t.Fatal("configured Wi-Fi without approval")
		}
	}
}

func TestRestoreOnlyRequestsAPRole(t *testing.T) {
	link := newFakeBluetooth()
	if err := testProvisioner(link).Restore(context.Background(), domain.Device{}); err != nil {
		t.Fatal(err)
	}
	if !link.closed || link.joinCount != 0 {
		t.Fatal("restore attempted a network join")
	}
	f := link.writes[len(link.writes)-1]
	if f.set != 7 || f.id != 0x48 || !bytes.Equal(f.payload, []byte{0}) {
		t.Fatal("restore did not select AP role")
	}
}

func TestNotificationAssemblerKeepsPartialFrames(t *testing.T) {
	encoded := encodeFrame(command(7, 7, 7, []byte{0, 1, 'x'}))
	var a notificationAssembler
	if len(a.append(encoded[:3])) != 0 || len(a.append(encoded[3:9])) != 0 {
		t.Fatal("emitted a partial frame")
	}
	frames := a.append(encoded[9:])
	if len(frames) != 1 || !bytes.Equal(frames[0].payload, []byte{0, 1, 'x'}) {
		t.Fatal("fragmented notification did not reassemble")
	}
	a.append(make([]byte, 4097))
	if len(a.pending) != 0 {
		t.Fatal("oversized notification retained")
	}
}

func FuzzBLENotifications(f *testing.F) {
	f.Add([]byte{0x55, 13, 4}, []byte{0, 1})
	f.Fuzz(func(t *testing.T, first, second []byte) {
		var a notificationAssembler
		a.append(first)
		a.append(second)
		if len(a.pending) > 4096 {
			t.Fatal("unbounded notification buffer")
		}
	})
}
