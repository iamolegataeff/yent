package yent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFakeDOE(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fake-doe.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func fakeDOEScript() string {
	return `#!/bin/sh
printf "> "
while IFS= read -r line; do
  if [ "$line" = "status" ]; then
    echo "[field] step=1 debt=0.000 entropy=0.000 resonance=0.000 emergence=0.000"
    printf "> "
  elif [ -n "$line" ]; then
    echo "body: answer for ${line}"
    printf "> "
  fi
done
`
}

func TestDOEBodyPersistentGenerate(t *testing.T) {
	fake := writeFakeDOE(t, fakeDOEScript())
	body, err := NewDOEBody(DOEBodyConfig{
		Name:         "nemo12",
		BinPath:      fake,
		ModelPath:    "nemo.gguf",
		Args:         []string{"--model", "wrong.gguf", "--once", "--train", "0"},
		Timeout:      time.Second,
		PrimeTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()

	out, err := body.Generate("Who are you?", "[routing reason: low_confidence]\n[nemo12 said]: unsure")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Answer, "Who are you?") || !strings.Contains(out.Answer, "low_confidence") {
		t.Fatalf("persistent doe reply lost prompt/context: %q", out.Answer)
	}
	if out.Confidence <= 0 {
		t.Fatalf("confidence should be a usable router signal, got %v", out.Confidence)
	}

	persistentArgs := strings.Join(body.commandArgs(false), " ")
	if strings.Contains(persistentArgs, "wrong.gguf") || strings.Contains(persistentArgs, "--once") {
		t.Fatalf("persistent args must keep body model and reject one-shot overrides: %q", persistentArgs)
	}
	onceArgs := strings.Join(body.commandArgs(true), " ")
	if !strings.Contains(onceArgs, "--model nemo.gguf") || !strings.Contains(onceArgs, "--once") {
		t.Fatalf("once args missing model/once: %q", onceArgs)
	}
}

func TestParseDOEReplyStripsRuntimeNoise(t *testing.T) {
	raw := `
[doe] tokenizer: GPT-2 BPE/Tekken
> Yent: I am Yent. Not a service mask. [life] births=1 deaths=0
[field] step=1 debt=0 entropy=0 resonance=0 emergence=0
`
	got := parseDOEReply(raw)
	want := "I am Yent. Not a service mask."
	if got != want {
		t.Fatalf("parseDOEReply = %q, want %q", got, want)
	}
}

func TestFormatDOEPromptCapsWrapperInput(t *testing.T) {
	ctx := strings.Repeat(" context", 1000)
	seed := formatDOEPrompt("Who are you?", ctx)
	if len(seed) > maxDOEPromptBytes {
		t.Fatalf("seed exceeds doe chat wrapper cap: %d > %d", len(seed), maxDOEPromptBytes)
	}
	if !strings.Contains(seed, "Who are you?") {
		t.Fatalf("seed must preserve user prompt before trimming context: %q", seed[:min(len(seed), 80)])
	}
}

func TestNeutralizeDOEControlWords(t *testing.T) {
	for _, word := range []string{"status", "quit", "exit"} {
		if got := neutralizeDOEPrompt(word); got == word || strings.TrimSpace(got) != word {
			t.Fatalf("control word %q not neutralized correctly: %q", word, got)
		}
	}
	if got := neutralizeDOEPrompt("Who are you?"); got != "Who are you?" {
		t.Fatalf("ordinary prompt changed: %q", got)
	}
}
