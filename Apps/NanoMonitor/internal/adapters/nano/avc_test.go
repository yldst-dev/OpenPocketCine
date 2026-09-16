package nano

import (
	"bytes"
	"testing"

	"nanomonitor/internal/domain"
)

func videoPackets(data []byte, sequence uint16, chunk int) [][]byte {
	header := make([]byte, 16)
	copy(header, []byte{0, 0, 1, 0xff})
	le.PutUint32(header[4:], uint32(len(data)))
	data = append(header, data...)
	var packets [][]byte
	for index := 0; len(data) > 0; index++ {
		n := min(chunk, len(data))
		body := make([]byte, 12)
		body[8] = byte(index / 63)
		body[9] = byte((index % 63 & 1) << 7)
		body[10] = byte(index % 63 / 2)
		packets = append(packets, packet(2, 1, sequence, append(body, data[:n]...)))
		data = data[n:]
		sequence += 8
	}
	return packets
}

func TestAssemblyCrossesGroupsAndSequenceWrap(t *testing.T) {
	data := bytes.Repeat([]byte{0x65, 0x88, 0x84, 0x21}, 25000)
	packets := videoPackets(data, 0xfff0, 1452)
	var a assembler
	for i, p := range packets {
		got, lost := a.feed(p)
		if lost {
			t.Fatal("contiguous frame marked lost")
		}
		if i == len(packets)-1 {
			if !bytes.Equal(got, data) {
				t.Fatal("multi-group picture was truncated")
			}
		} else if got != nil {
			t.Fatal("emitted before declared length")
		}
		if duplicate, lost := a.feed(p); duplicate != nil || lost {
			t.Fatal("duplicate affected assembly")
		}
	}
}

func TestLossDropsPictureAndNextMarkerRecovers(t *testing.T) {
	packets := videoPackets(bytes.Repeat([]byte{0x65}, 6000), 0, 1000)
	var a assembler
	a.feed(packets[0])
	if data, lost := a.feed(packets[2]); data != nil || !lost {
		t.Fatal("missing fragment was accepted")
	}
	for _, p := range packets[3:] {
		if data, _ := a.feed(p); data != nil {
			t.Fatal("emitted a corrupt picture")
		}
	}
	want := []byte{0, 0, 1, 0x65, 1}
	got, _ := a.feed(videoPackets(want, uint16(len(packets)*8), 1000)[0])
	if !bytes.Equal(got, want) {
		t.Fatal("did not recover at next complete marker")
	}
}

func TestPrivateMetadataCannotCreateFakeNAL(t *testing.T) {
	metadata := append([]byte{0, 0, 1, 6, 0xf0, 25}, make([]byte, 25)...)
	copy(metadata[8:], []byte{0, 0, 1, 0x65, 0x88})
	metadata = append(metadata, 0x80)
	data := append(metadata, []byte{0, 0, 0, 1, 0x41, 0x11}...)
	got, err := nals(data)
	if err != nil || len(got) != 1 || !bytes.Equal(got[0], []byte{0x41, 0x11}) {
		t.Fatalf("private metadata escaped: %x %v", got, err)
	}
	if _, err := nals(metadata[:20]); err == nil {
		t.Fatal("accepted truncated private metadata")
	}
}

func TestAVCGateKeepsParametersButWaitsForIDR(t *testing.T) {
	var gate avcGate
	params := fromHex(t, "000000016764001facb402802dd3501040106d0a13500000000168ee06f2c0")
	if data, err := gate.accept(params); data != nil || err != nil {
		t.Fatal("parameter-only AU should not be a picture")
	}
	if data, err := gate.accept([]byte{0, 0, 1, 0x41, 1}); data != nil || err != nil {
		t.Fatal("P slice must wait for IDR")
	}
	idr := []byte{0, 0, 1, 0x65, 1}
	data, err := gate.accept(idr)
	units, _ := nals(data)
	if err != nil || len(units) != 3 || !gate.ready {
		t.Fatalf("IDR bootstrap failed: %x %v", data, err)
	}
	gate.ready = false
	if data, err := gate.accept(idr); err != nil || len(data) == 0 {
		t.Fatal("resync must reuse cached parameters")
	}
	if _, err := gate.accept([]byte{0, 0, 1, 0x40, 1}); err == nil {
		t.Fatal("accepted HEVC")
	}
}

func TestOversizedDeclarationIsRejected(t *testing.T) {
	p := videoPackets([]byte{1}, 0, 100)[0]
	le.PutUint32(p[24:], domain.MaxAccessUnit+1)
	var a assembler
	if data, lost := a.feed(p); data != nil || !lost || cap(a.buffer) != 0 {
		t.Fatal("oversized declaration allocated a frame")
	}
}

func FuzzAVC(f *testing.F) {
	f.Add([]byte{0, 0, 1, 0x65, 0x88})
	f.Add([]byte{0, 0, 1, 6, 0xf0, 25})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 65536 {
			return
		}
		var gate avcGate
		_, _ = gate.accept(data)
		var a assembler
		_, _ = a.feed(data)
	})
}
