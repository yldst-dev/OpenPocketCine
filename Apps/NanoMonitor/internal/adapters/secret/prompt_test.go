package secret

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPromptScriptUsesArgumentsAndCompiles(t *testing.T) {
	for _, hidden := range []bool{true, false} {
		script := promptScript(hidden)
		if !strings.Contains(script, "item 1 of argv") || strings.Contains(script, "test-only-password") ||
			strings.Contains(script, "with hidden answer") != hidden {
			t.Fatal("unsafe prompt script")
		}
		if runtime.GOOS == "darwin" {
			path := filepath.Join(t.TempDir(), "prompt.scpt")
			if output, err := exec.Command("osacompile", "-o", path, "-e", script).CombinedOutput(); err != nil {
				t.Fatalf("prompt syntax: %v: %s", err, output)
			}
		}
	}
}
