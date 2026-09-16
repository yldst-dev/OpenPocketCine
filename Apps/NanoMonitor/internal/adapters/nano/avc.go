package nano

import (
	"bytes"
	"errors"

	"nanomonitor/internal/domain"
)

type assembler struct {
	expected int
	buffer   []byte
	last     uint16
	hasLast  bool
}

func (a *assembler) reset() {
	a.expected = 0
	a.buffer = nil
	a.hasLast = false
}

func (a *assembler) feed(p []byte) ([]byte, bool) {
	if len(p) <= 20 || p[6] != 2 {
		return nil, false
	}
	seq := le.Uint16(p[4:6])
	if a.hasLast && seq == a.last {
		return nil, false
	}
	lost := a.hasLast && seq != a.last+8
	a.last, a.hasLast = seq, true
	if lost {
		a.expected, a.buffer = 0, nil
	}
	body := p[20:]
	if bytes.HasPrefix(body, []byte{0, 0, 1, 0xff}) {
		lost = lost || a.expected != 0
		a.expected, a.buffer = 0, nil
		if len(body) < 16 {
			return nil, true
		}
		size := le.Uint32(body[4:8])
		if size == 0 || size > domain.MaxAccessUnit {
			return nil, true
		}
		a.expected = int(size) + 16
	}
	if a.expected == 0 {
		return nil, lost
	}
	if len(body) > a.expected-len(a.buffer) {
		a.expected, a.buffer = 0, nil
		return nil, true
	}
	a.buffer = append(a.buffer, body...)
	if len(a.buffer) != a.expected {
		return nil, lost
	}
	out := a.buffer[16:]
	a.expected, a.buffer = 0, nil
	return out, lost
}

func nals(data []byte) ([][]byte, error) {
	var result [][]byte
	start := -1
	for i := 0; i+3 <= len(data); {
		if data[i] != 0 || data[i+1] != 0 || data[i+2] != 1 {
			i++
			continue
		}
		if start >= 0 {
			if nal := bytes.TrimRight(data[start:i], "\x00"); len(nal) > 0 {
				result = append(result, nal)
			}
		}
		start = i + 3
		if i+6 <= len(data) && bytes.Equal(data[i+3:i+6], []byte{6, 0xf0, 25}) {
			if i+32 > len(data) || data[i+31] != 0x80 {
				return nil, errors.New("Nano 영상 메타데이터가 잘렸습니다")
			}
			start = -1
			i += 32
		} else {
			i += 3
		}
	}
	if start >= 0 {
		if nal := bytes.TrimRight(data[start:], "\x00"); len(nal) > 0 {
			result = append(result, nal)
		}
	}
	return result, nil
}

type avcGate struct {
	sps, pps []byte
	ready    bool
}

func (g *avcGate) accept(data []byte) ([]byte, error) {
	units, err := nals(data)
	if err != nil {
		return nil, err
	}
	var picture, idr bool
	for _, nal := range units {
		if nal[0] == 0x40 || nal[0] == 0x42 || nal[0] == 0x44 {
			return nil, errors.New("AVC 영상이 아닙니다. 이 프로그램은 Osmo Nano만 지원합니다")
		}
		switch nal[0] & 31 {
		case 7, 8:
			if len(nal) > 65536 {
				return nil, errors.New("영상 초기화 데이터가 너무 큽니다")
			}
			if nal[0]&31 == 7 {
				if !bytes.Equal(g.sps, nal) {
					g.ready = false
				}
				g.sps = bytes.Clone(nal)
			} else {
				if !bytes.Equal(g.pps, nal) {
					g.ready = false
				}
				g.pps = bytes.Clone(nal)
			}
		case 5:
			idr, picture = true, true
		case 1, 2, 3, 4:
			picture = true
		}
	}
	if !picture || len(g.sps) == 0 || len(g.pps) == 0 || (!g.ready && !idr) {
		return nil, nil
	}
	var output []byte
	appendNAL := func(nal []byte) {
		output = append(output, 0, 0, 0, 1)
		output = append(output, nal...)
	}
	if idr {
		appendNAL(g.sps)
		appendNAL(g.pps)
	}
	for _, nal := range units {
		if nal[0]&31 != 7 && nal[0]&31 != 8 {
			appendNAL(nal)
		}
	}
	if len(output) > domain.MaxAccessUnit {
		return nil, errors.New("영상 프레임이 허용 크기를 넘었습니다")
	}
	g.ready = true
	return output, nil
}
