package yent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	doeStatusCmd       = "status"
	defaultDOETimeout  = 45 * time.Second
	defaultDOEPrime    = 90 * time.Second
	maxDOEPromptBytes  = 1800 // doe.c wraps chat prompts into a 2048-byte buffer.
	doeScannerMaxBytes = 4 << 20
)

// DOEBodyConfig describes one process-backed inference body. The Go router does
// not embed the model; it keeps a doe_field REPL resident and talks over
// stdin/stdout. Args are passed after "--model <ModelPath>"; "--once" and "--model"
// inside Args are ignored so the persistent daemon cannot be accidentally made
// one-shot or pointed at another body.
type DOEBodyConfig struct {
	Name      string
	BinPath   string
	ModelPath string
	WorkDir   string
	Args      []string
	Env       []string

	Timeout      time.Duration
	PrimeTimeout time.Duration

	Confidence func(answer string) float64
	Verdict    func(answer string) *Verdict
}

// DOEBody is a real Body backed by a persistent doe process.
type DOEBody struct {
	cfg DOEBodyConfig

	mu     sync.Mutex
	daemon *doeProcess
}

// NewDOEBody builds a process-backed router body. The process starts lazily on
// first Generate so callers can register multiple bodies without loading both.
func NewDOEBody(cfg DOEBodyConfig) (*DOEBody, error) {
	if strings.TrimSpace(cfg.Name) == "" {
		return nil, errors.New("doe body name is required")
	}
	if strings.TrimSpace(cfg.BinPath) == "" {
		return nil, errors.New("doe body binary path is required")
	}
	if strings.TrimSpace(cfg.ModelPath) == "" {
		return nil, errors.New("doe body model path is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = defaultDOETimeout
	}
	if cfg.PrimeTimeout <= 0 {
		cfg.PrimeTimeout = defaultDOEPrime
	}
	return &DOEBody{cfg: cfg}, nil
}

func (b *DOEBody) Name() string { return b.cfg.Name }

// Generate sends one prompt through the resident doe REPL. If the daemon dies
// before the status sentinel, the same prompt is attempted once through --once.
func (b *DOEBody) Generate(prompt, ctx string) (BodyResult, error) {
	if b == nil {
		return BodyResult{}, errors.New("nil doe body")
	}
	seed := formatDOEPrompt(prompt, ctx)
	if seed == "" {
		return BodyResult{}, errors.New("empty doe prompt")
	}
	b.mu.Lock()
	defer b.mu.Unlock()

	daemonReady := b.ensureDaemonLocked() == nil && b.daemon != nil && !b.daemon.dead
	genCtx, cancel := context.WithTimeout(context.Background(), b.cfg.Timeout)
	defer cancel()

	if daemonReady {
		if raw, ok := b.daemon.exchange(genCtx, seed); ok {
			if answer := parseDOEReply(raw); answer != "" {
				return b.result(answer), nil
			}
		}
	}
	if genCtx.Err() != nil {
		return BodyResult{}, genCtx.Err()
	}
	answer, err := b.runOnce(genCtx, seed)
	if err != nil {
		return BodyResult{}, err
	}
	return b.result(answer), nil
}

// Close stops the resident doe process, if one was started.
func (b *DOEBody) Close() error {
	if b == nil {
		return nil
	}
	b.mu.Lock()
	d := b.daemon
	b.daemon = nil
	b.mu.Unlock()
	if d != nil {
		d.close()
	}
	return nil
}

func (b *DOEBody) result(answer string) BodyResult {
	conf := EstimateBodyConfidence(answer)
	if b.cfg.Confidence != nil {
		conf = b.cfg.Confidence(answer)
	}
	return BodyResult{
		Answer:     answer,
		Confidence: conf,
		Verdict:    parseVerdictHook(b.cfg.Verdict, answer),
	}
}

func parseVerdictHook(fn func(string) *Verdict, answer string) *Verdict {
	if fn == nil {
		return nil
	}
	return fn(answer)
}

func (b *DOEBody) ensureDaemonLocked() error {
	if b.daemon != nil && !b.daemon.dead {
		return nil
	}
	if b.daemon != nil {
		b.daemon.close()
		b.daemon = nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), b.cfg.PrimeTimeout)
	defer cancel()
	d, err := b.startProcess(false)
	if err != nil {
		return err
	}
	if _, ok := d.exchange(ctx, ""); !ok {
		d.close()
		return errors.New("doe daemon did not reach status sentinel")
	}
	b.daemon = d
	return nil
}

func (b *DOEBody) runOnce(ctx context.Context, seed string) (string, error) {
	cmd := exec.CommandContext(ctx, b.cfg.BinPath, b.commandArgs(true)...)
	if b.cfg.WorkDir != "" {
		cmd.Dir = b.cfg.WorkDir
	}
	if len(b.cfg.Env) > 0 {
		cmd.Env = append(os.Environ(), b.cfg.Env...)
	}
	cmd.Stdin = strings.NewReader(seed + "\n")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("doe once: %w", err)
	}
	answer := parseDOEReply(string(out))
	if answer == "" {
		return "", errors.New("doe once produced no parseable answer")
	}
	return answer, nil
}

func (b *DOEBody) startProcess(once bool) (*doeProcess, error) {
	cmd := exec.Command(b.cfg.BinPath, b.commandArgs(once)...)
	if b.cfg.WorkDir != "" {
		cmd.Dir = b.cfg.WorkDir
	}
	if len(b.cfg.Env) > 0 {
		cmd.Env = append(os.Environ(), b.cfg.Env...)
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	outPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = in.Close()
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		_ = in.Close()
		return nil, err
	}
	sc := bufio.NewScanner(outPipe)
	sc.Buffer(make([]byte, 64*1024), doeScannerMaxBytes)
	return &doeProcess{cmd: cmd, in: in, out: sc}, nil
}

func (b *DOEBody) commandArgs(once bool) []string {
	args := []string{"--model", b.cfg.ModelPath}
	for i := 0; i < len(b.cfg.Args); i++ {
		a := b.cfg.Args[i]
		if a == "--model" {
			i++ // ModelPath is the body source of truth.
			continue
		}
		if a == "--once" {
			continue
		}
		args = append(args, a)
	}
	if once {
		args = append(args, "--once")
	}
	return args
}

type doeProcess struct {
	cmd    *exec.Cmd
	in     io.WriteCloser
	out    *bufio.Scanner
	dead   bool
	reaped sync.Once
}

func (d *doeProcess) exchange(ctx context.Context, seed string) (string, bool) {
	if d == nil || d.dead {
		return "", false
	}
	if _, err := fmt.Fprintf(d.in, "%s\n%s\n", neutralizeDOEPrompt(seed), doeStatusCmd); err != nil {
		d.dead = true
		d.reap()
		return "", false
	}
	type reply struct {
		text string
		ok   bool
	}
	ch := make(chan reply, 1)
	go func() {
		var b strings.Builder
		ok := false
		for d.out.Scan() {
			line := d.out.Text()
			if isDOEStatusSentinel(line) {
				ok = true
				break
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
		ch <- reply{text: b.String(), ok: ok}
	}()
	select {
	case r := <-ch:
		if !r.ok {
			d.dead = true
			d.reap()
			return "", false
		}
		return r.text, true
	case <-ctx.Done():
		d.dead = true
		d.kill()
		<-ch
		d.reap()
		return "", false
	}
}

func (d *doeProcess) close() {
	if d == nil {
		return
	}
	if d.in != nil {
		_ = d.in.Close()
	}
	done := make(chan struct{})
	go func() {
		d.reap()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		d.kill()
		<-done
	}
}

func (d *doeProcess) kill() {
	if d != nil && d.cmd != nil && d.cmd.Process != nil {
		_ = d.cmd.Process.Kill()
	}
}

func (d *doeProcess) reap() {
	if d != nil {
		d.reaped.Do(func() { _ = d.cmd.Wait() })
	}
}

func isDOEStatusSentinel(line string) bool {
	t := strings.TrimLeft(line, "> \t")
	return strings.HasPrefix(t, "[field] step=") &&
		strings.Contains(t, "debt=") &&
		strings.Contains(t, "entropy=") &&
		strings.Contains(t, "resonance=") &&
		strings.Contains(t, "emergence=")
}

func neutralizeDOEPrompt(seed string) string {
	switch seed {
	case doeStatusCmd, "quit", "exit":
		return " " + seed
	default:
		return seed
	}
}

func formatDOEPrompt(prompt, ctx string) string {
	prompt = strings.Join(strings.Fields(strings.TrimSpace(prompt)), " ")
	ctx = strings.Join(strings.Fields(strings.TrimSpace(ctx)), " ")
	var seed string
	if ctx == "" {
		seed = prompt
	} else if isRouteContext(ctx) {
		seed = formatContextualDOEPrompt(prompt, ctx)
	} else {
		seed = formatPrimerDOEPrompt(prompt, ctx)
	}
	if len(seed) <= maxDOEPromptBytes {
		return neutralizeDOEPrompt(seed)
	}
	cut := maxDOEPromptBytes
	if sp := strings.LastIndexByte(seed[:cut], ' '); sp > 0 {
		cut = sp
	}
	return neutralizeDOEPrompt(strings.ToValidUTF8(seed[:cut], ""))
}

func isRouteContext(ctx string) bool {
	return strings.Contains(ctx, "[router fact]") ||
		strings.Contains(ctx, "[routing reason") ||
		strings.Contains(ctx, "[context facts]")
}

func formatPrimerDOEPrompt(prompt, primer string) string {
	const promptPrefix = " Human asks: "
	suffix := promptPrefix + prompt
	budget := maxDOEPromptBytes - len(suffix) - 1
	if budget <= 0 {
		return prompt
	}
	if primer = truncateAtWord(primer, budget); primer == "" {
		return prompt
	}
	return primer + suffix
}

func formatContextualDOEPrompt(prompt, ctx string) string {
	const (
		contextPrefix = "[context facts]: "
		contract      = " [answer contract]: Answer the human prompt directly. Use context as private factual evidence. If the human asks about route or body facts, use [router fact] literally. Do not make routing or context the subject unless the human asks."
		promptPrefix  = " [human prompt]: "
	)
	suffix := contract + promptPrefix + prompt
	budget := maxDOEPromptBytes - len(contextPrefix) - len(suffix)
	if budget < 0 {
		return strings.TrimSpace(suffix)
	}
	return contextPrefix + truncateAtWord(ctx, budget) + suffix
}

func truncateAtWord(s string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(s) <= maxBytes {
		return s
	}
	cut := maxBytes
	if sp := strings.LastIndexByte(s[:cut], ' '); sp > 0 {
		cut = sp
	}
	return strings.TrimSpace(strings.ToValidUTF8(s[:cut], ""))
}

func parseDOEReply(out string) string {
	var b strings.Builder
	capturing := false
	seenPrompt := false
	for _, line := range strings.Split(out, "\n") {
		t := strings.TrimSpace(line)
		if !capturing {
			if strings.HasPrefix(t, ">") {
				seenPrompt = true
				body := strings.TrimSpace(strings.TrimPrefix(t, ">"))
				if body == "" || strings.HasPrefix(body, "[") {
					continue
				}
				capturing = true
				b.WriteString(body)
				b.WriteByte(' ')
				continue
			}
			if !seenPrompt || t == "" || strings.HasPrefix(t, "[") {
				continue
			}
			capturing = true
			b.WriteString(t)
			b.WriteByte(' ')
			continue
		}
		if t == "" || strings.HasPrefix(t, "[") || strings.HasPrefix(t, ">") {
			break
		}
		b.WriteString(t)
		b.WriteByte(' ')
	}
	answer := strings.TrimSpace(b.String())
	if i := strings.Index(answer, "[life]"); i >= 0 {
		answer = strings.TrimSpace(answer[:i])
	}
	answer = stripDOELabel(answer)
	answer = strings.ToValidUTF8(answer, "")
	return strings.Join(strings.Fields(answer), " ")
}

func stripDOELabel(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	colon := strings.IndexByte(s, ':')
	if colon <= 0 || colon > 32 {
		return s
	}
	label := s[:colon]
	for _, r := range label {
		if !(r == '_' || r == '-' || r == ' ' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z') {
			return s
		}
	}
	return strings.TrimSpace(s[colon+1:])
}

// EstimateBodyConfidence is a cheap runtime signal for router v1. It is not a
// claim about model quality; it only detects empty, invalid, extremely short, or
// repetition-heavy output before the router sees a model-native entropy signal.
func EstimateBodyConfidence(answer string) float64 {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return 0
	}
	score := 0.35
	if utf8.ValidString(answer) {
		score += 0.1
	}
	runes := utf8.RuneCountInString(answer)
	switch {
	case runes > 160:
		score += 0.3
	case runes > 60:
		score += 0.2
	case runes > 16:
		score += 0.1
	default:
		score -= 0.15
	}
	if strings.ContainsAny(answer, ".?!") {
		score += 0.1
	}
	if repetitionRatio(answer) > 0.45 {
		score -= 0.3
	}
	if strings.ContainsRune(answer, '\uFFFD') {
		score -= 0.3
	}
	return clamp01(score)
}

func repetitionRatio(s string) float64 {
	words := strings.Fields(strings.ToLower(s))
	if len(words) < 6 {
		return 0
	}
	counts := map[string]int{}
	maxCount := 0
	for _, w := range words {
		counts[w]++
		if counts[w] > maxCount {
			maxCount = counts[w]
		}
	}
	return math.Max(0, float64(maxCount-1)/float64(len(words)))
}
