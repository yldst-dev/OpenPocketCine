package nano

import (
	"encoding/binary"
	"errors"
)

var le = binary.LittleEndian

type frame struct {
	sender, receiver byte
	seq              uint16
	flags, set, id   byte
	payload          []byte
}

func crc8(data []byte) byte {
	c := byte(0x77)
	for _, b := range data {
		c ^= b
		for range 8 {
			if c&1 != 0 {
				c = c>>1 ^ 0x8c
			} else {
				c >>= 1
			}
		}
	}
	return c
}

func crc16(data []byte) uint16 {
	c := uint16(0x3692)
	for _, b := range data {
		c ^= uint16(b)
		for range 8 {
			if c&1 != 0 {
				c = c>>1 ^ 0x8408
			} else {
				c >>= 1
			}
		}
	}
	return c
}

func encodeFrame(f frame) []byte {
	n := 13 + len(f.payload)
	if n > 1023 {
		return nil
	}
	b := make([]byte, n)
	b[0], b[1], b[2] = 0x55, byte(n), byte(4|(n>>8))
	b[3] = crc8(b[:3])
	b[4], b[5] = f.sender, f.receiver
	le.PutUint16(b[6:8], f.seq)
	b[8], b[9], b[10] = f.flags, f.set, f.id
	copy(b[11:], f.payload)
	le.PutUint16(b[n-2:], crc16(b[:n-2]))
	return b
}

func scanFrames(data []byte) []frame {
	var result []frame
	for i := 0; i+13 <= len(data); {
		b := data[i:]
		n := int(b[1]) | int(b[2]&3)<<8
		if b[0] != 0x55 || b[2]>>2 != 1 || n < 13 || n > len(b) ||
			crc8(b[:3]) != b[3] || crc16(b[:n-2]) != le.Uint16(b[n-2:n]) {
			i++
			continue
		}
		result = append(result, frame{b[4], b[5], le.Uint16(b[6:8]), b[8], b[9], b[10], b[11 : n-2]})
		i += n
	}
	return result
}

func packet(kind byte, session, seq uint16, payload []byte) []byte {
	n := len(payload) + 8
	if n > 0x3fff {
		return nil
	}
	b := make([]byte, n)
	le.PutUint16(b, 0x8000|uint16(n))
	le.PutUint16(b[2:4], session)
	le.PutUint16(b[4:6], seq)
	b[6] = kind
	for _, v := range b[:7] {
		b[7] ^= v
	}
	copy(b[8:], payload)
	return b
}

func validPacket(b []byte) bool {
	if len(b) < 8 || len(b) > 0x3fff || le.Uint16(b)&0xc000 != 0x8000 ||
		int(le.Uint16(b)&0x3fff) != len(b) {
		return false
	}
	var xor byte
	for _, v := range b[:8] {
		xor ^= v
	}
	return xor == 0 && b[6] <= 5
}

func handshake(base uint16) []byte {
	b := []byte{
		0, 0, 0x64, 0, 0x64, 0, 0xc0, 5, 0x14, 0, 0, 0x64, 0, 0,
		1, 0x90, 1, 0xc0, 5, 0x14, 0, 0, 0x64, 0, 0x14, 0, 0x64, 0, 0xc0, 5,
		0x14, 0, 0, 0x64, 0, 1, 1, 4, 1, 2,
	}
	le.PutUint16(b, base)
	return b
}

func command(receiver, set, id byte, payload []byte) frame {
	return frame{sender: 2, receiver: receiver, flags: 0x40, set: set, id: id, payload: payload}
}

func pairing() frame {
	identifier := "284ae5b8d76b3375a04a6417ad71bea3"
	payload := append([]byte{byte(len(identifier))}, identifier...)
	payload = append(payload, 4, 'o', 's', 'm', 'o')
	f := command(7, 7, 0x45, payload)
	f.seq = 0x8092
	return f
}

func deviceInfo() frame {
	b := make([]byte, 62)
	copy(b[1:], "APP")
	b[41], b[50], b[51] = 2, 2, 8
	f := command(0x48, 0, 0x81, b)
	f.flags = 0x80
	return f
}

func presence() frame {
	return command(0x28, 0, 0x88, []byte{0x17, 0, 0x46, 0x23, 0x7c, 0x41, 0x50, 0x50, 0, 0, 0, 0, 0, 2})
}

func subscription(key string, id uint32) frame {
	b := make([]byte, 15+len(key)+4)
	b[0], b[1] = 2, 2
	le.PutUint32(b[4:8], id)
	le.PutUint16(b[11:13], uint16(len(key)+6))
	le.PutUint16(b[13:15], uint16(len(key)))
	copy(b[15:], key)
	return command(0x28, 0, 0x99, b)
}

func previewGate(start bool) frame {
	b := make([]byte, 11)
	b[10] = 4
	if start {
		b[10] = 3
	}
	return command(1, 2, 9, b)
}

func enablePreview() frame {
	return command(0x41, 9, 0xa8, []byte{0, 4, 2, 0, 0, 0, 0, 0, 0, 0})
}

func cameraName(payload []byte) (string, error) {
	if len(payload) < 3 || payload[0] != 0 || payload[1] == 0 || int(payload[1])+2 > len(payload) {
		return "", errors.New("카메라 식별 응답이 올바르지 않습니다")
	}
	return string(payload[2 : 2+int(payload[1])]), nil
}

type windows struct {
	video, data, extra uint16
	hasVideo, hasData  bool
}

func (w *windows) observe(p []byte) {
	switch p[6] {
	case 1:
		if len(p) == 34 {
			if !w.hasVideo {
				w.video = le.Uint16(p[10:12])
			}
			if !w.hasData {
				w.data = le.Uint16(p[18:20])
			}
			w.extra = le.Uint16(p[26:28])
		}
	case 2:
		seq := le.Uint16(p[4:6])
		if !w.hasVideo || sequenceAfter(seq, w.video) {
			w.video, w.hasVideo = seq, true
		}
	case 3:
		seq := le.Uint16(p[4:6])
		if !w.hasData || sequenceAfter(seq, w.data) {
			w.data, w.hasData = seq, true
		}
	}
}

func (w windows) payload() []byte {
	b := make([]byte, 26)
	for i, v := range []uint16{w.video, w.data, w.extra} {
		le.PutUint16(b[i*8:], v)
		le.PutUint16(b[i*8+2:], v)
	}
	return b
}

func sequenceAfter(next, previous uint16) bool {
	distance := next - previous
	return distance != 0 && distance < 0x8000
}
