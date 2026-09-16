package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestHelpAndInputValidationDoNotOpenNetwork(t *testing.T) {
	var output bytes.Buffer
	err := run(context.Background(), []string{"-h"}, &output, &output)
	if !errors.Is(err, flag.ErrHelp) || !strings.Contains(output.String(), "-camera") {
		t.Fatalf("help failed: %v", err)
	}
	for _, args := range [][]string{{"-camera", "8.8.8.8"}, {"unexpected"}, {"-discover", "-camera", "192.168.1.2"}} {
		if err := run(context.Background(), args, &output, &output); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
