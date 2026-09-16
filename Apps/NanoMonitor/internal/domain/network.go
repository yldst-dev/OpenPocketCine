package domain

import (
	"bytes"
	"errors"
	"unicode/utf8"
)

type Device struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Network struct {
	SSID       string
	Passphrase []byte
}

func (n Network) Validate() error {
	if len(n.SSID) == 0 || len(n.SSID) > 32 || !utf8.ValidString(n.SSID) ||
		bytes.ContainsAny([]byte(n.SSID), "\x00\r\n") {
		return errors.New("공유기 이름은 줄바꿈 없이 1~32바이트여야 합니다")
	}
	if len(n.Passphrase) < 8 || len(n.Passphrase) > 63 || !utf8.Valid(n.Passphrase) ||
		bytes.ContainsAny(n.Passphrase, "\x00\r\n") {
		return errors.New("WPA2 비밀번호는 줄바꿈 없이 8~63바이트여야 합니다")
	}
	return nil
}

type CameraIdentity struct {
	Name string
}

var ErrJoinUnconfirmed = errors.New("공유기 접속 응답을 받지 못했습니다")
