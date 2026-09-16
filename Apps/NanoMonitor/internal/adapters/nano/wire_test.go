package nano

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func fromHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestGoldenWireFrames(t *testing.T) {
	f := command(1, 2, 0x0c, []byte{1, 1, 0, 1})
	f.seq = 0xa000
	want := fromHex(t, "55110492020100a040020c01010001b63b")
	if got := encodeFrame(f); !bytes.Equal(got, want) {
		t.Fatalf("frame: %x", got)
	}
	header := packet(5, 0x3a7c, 0xb890, make([]byte, 29))[:8]
	if !bytes.Equal(header, fromHex(t, "25807c3a90b805ce")) {
		t.Fatalf("transport header: %x", header)
	}
	if got := packet(0, 0x1234, 0, handshake(0xb887)); !bytes.Equal(got[:8], fromHex(t, "3080341200000096")) || len(got) != 48 {
		t.Fatalf("handshake: %x", got)
	}
	encoded := encodeFrame(pairing())
	if len(encoded) != 51 || !bytes.Equal(encoded[len(encoded)-2:], []byte{0xa0, 0xb4}) {
		t.Fatalf("pairing vector: %x", encoded)
	}
}

func TestDecodeRejectsCorruptFrames(t *testing.T) {
	f := command(7, 7, 7, []byte{0, 4, 'N', 'a', 'n', 'o'})
	encoded := encodeFrame(f)
	if got := scanFrames(append([]byte{1, 2, 3}, encoded...)); len(got) != 1 || !bytes.Equal(got[0].payload, f.payload) {
		t.Fatal("valid frame was lost")
	}
	for i := range encoded {
		bad := bytes.Clone(encoded)
		bad[i] ^= 0x80
		if len(scanFrames(bad)) != 0 {
			t.Fatalf("accepted corruption at byte %d", i)
		}
	}
	for n := range len(encoded) {
		if len(scanFrames(encoded[:n])) != 0 {
			t.Fatalf("accepted truncated frame of length %d", n)
		}
	}
}

func TestAckWindowsPreserveZeroAndDoNotRewind(t *testing.T) {
	w := windows{video: 100, data: 200, extra: 300}
	telemetry := packet(1, 1, 0, make([]byte, 26))
	le.PutUint16(telemetry[10:], 8)
	le.PutUint16(telemetry[18:], 16)
	le.PutUint16(telemetry[26:], 24)
	w.observe(telemetry)
	w.observe(packet(2, 1, 0, make([]byte, 20)))
	w.observe(packet(3, 1, 0, nil))
	w.observe(telemetry)
	if w.video != 0 || w.data != 0 || w.extra != 24 {
		t.Fatalf("rewound windows: %+v", w)
	}
	p := w.payload()
	if len(p) != 26 || le.Uint16(p[16:]) != 24 || le.Uint16(p[18:]) != 24 {
		t.Fatalf("bad ACK: %x", p)
	}
}

func TestPacketBoundsAndIdentity(t *testing.T) {
	p := packet(1, 10, 20, make([]byte, 26))
	if !validPacket(p) || validPacket(p[:33]) || validPacket(append(bytes.Clone(p), 0)) {
		t.Fatal("transport length validation")
	}
	p[7] ^= 1
	if validPacket(p) {
		t.Fatal("accepted invalid XOR")
	}
	for _, b := range [][]byte{nil, {0}, {0, 4, 'a'}, {0xe0, 1, 'a'}, {0, 0}} {
		if _, err := cameraName(b); err == nil {
			t.Fatalf("accepted invalid identity %x", b)
		}
	}
}

func FuzzWire(f *testing.F) {
	f.Add([]byte{0x55, 13, 4})
	f.Add(packet(0, 1, 0, handshake(100)))
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = validPacket(data)
		_ = scanFrames(data)
		_, _ = cameraName(data)
	})
}

func TestLatePacketsDoNotRewindAckWindowsAcrossWrap(t *testing.T) {
	var w windows
	for _, kind := range []byte{2, 3} {
		for _, sequence := range []uint16{0xfff0, 0xfff8, 0, 8, 0xfff0, 0} {
			w.observe(packet(kind, 1, sequence, nil))
		}
	}
	if w.video != 8 || w.data != 8 {
		t.Fatalf("old packets rewound ACK cursors: %+v", w)
	}
}
