package domain

import "time"

const MaxAccessUnit = 4 * 1024 * 1024

type AccessUnit struct {
	Data []byte
}

type RecoveryAction int

const (
	KeepReceiving RecoveryAction = iota
	RequestPicture
	StopStream
)

func Recovery(now, lastEnable, lastPicture time.Time, waitingForIDR bool, attempts int) RecoveryAction {
	if now.Sub(lastEnable) < 8*time.Second {
		return KeepReceiving
	}
	if !waitingForIDR && !lastPicture.IsZero() && now.Sub(lastPicture) < 2*time.Second {
		return KeepReceiving
	}
	if attempts >= 2 {
		return StopStream
	}
	return RequestPicture
}
