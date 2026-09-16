package domain

import (
	"strings"
	"testing"
)

func TestNetworkValidationUsesWireByteLimits(t *testing.T) {
	for _, network := range []Network{
		{SSID: "", Passphrase: []byte("test-only-password")},
		{SSID: strings.Repeat("가", 11), Passphrase: []byte("test-only-password")},
		{SSID: "TestNetwork", Passphrase: []byte("short")},
		{SSID: "TestNetwork", Passphrase: []byte(strings.Repeat("x", 64))},
		{SSID: "Test\x00Network", Passphrase: []byte("test-only-password")},
		{SSID: "TestNetwork", Passphrase: []byte("password\n")},
	} {
		if err := network.Validate(); err == nil {
			t.Fatal("invalid credentials were accepted")
		} else if strings.Contains(err.Error(), "test-only-password") {
			t.Fatal("validation error leaked a secret")
		}
	}
	if err := (Network{SSID: " TestNetwork ", Passphrase: []byte(" test-only-password ")}).Validate(); err != nil {
		t.Fatal("legitimate spaces were rejected")
	}
}
