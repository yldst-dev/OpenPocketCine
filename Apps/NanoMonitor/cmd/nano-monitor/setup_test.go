package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"testing"

	"nanomonitor/internal/domain"
)

func TestSetupHelpAndValidationDoNotOpenBluetooth(t *testing.T) {
	var output bytes.Buffer
	if err := run(context.Background(), []string{"setup", "-h"}, &output, &output); !errors.Is(err, flag.ErrHelp) {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"setup", "-list", "-restore-ap"}, {"setup", "-restore-ap", "-monitor"}, {"setup", "-password", "not-accepted"}} {
		if err := run(context.Background(), args, &output, &output); err == nil {
			t.Fatal("unsafe setup flags accepted")
		}
	}
}

func TestAmbiguousDevicesRequireExplicitSelection(t *testing.T) {
	devices := []domain.Device{{ID: "one", Name: "OsmoNano-TEST"}, {ID: "two", Name: "OsmoNano-TEST"}}
	if _, err := selectDevice(devices, ""); err == nil {
		t.Fatal("automatically selected among multiple cameras")
	}
	if _, err := selectDevice(devices, "OsmoNano-TEST"); err == nil {
		t.Fatal("duplicate names bypassed identity selection")
	}
	if device, err := selectDevice(devices, "two"); err != nil || device.ID != "two" {
		t.Fatal("explicit identity was not selected")
	}
}

func TestPrintedCommandsQuoteUntrustedNames(t *testing.T) {
	if got := shellQuote("OsmoNano-$(command)'value"); got != "'OsmoNano-$(command)'\\''value'" {
		t.Fatalf("unsafe quoting: %s", got)
	}
}
