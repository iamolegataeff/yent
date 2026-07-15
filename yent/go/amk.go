package yent

// amk.go — CGO bridge to AMK (Arianna Method Kernel)
//
// AML is the nervous system. Delta Voice is the mouth.
// Without the kernel, Yent is a voice without a brain.
//
// "from ariannamethod import Destiny"

/*
#cgo CFLAGS: -I${SRCDIR}/../c
#cgo LDFLAGS: ${SRCDIR}/../c/libamk.a
#include "ariannamethod.h"
#include <stdlib.h>
*/
import "C"
import (
	"fmt"
	"os"
	"strings"
	"sync"
	"unsafe"
)

// AMK wraps the Arianna Method Kernel (C shared library)
type AMK struct {
	mu      sync.Mutex
	running bool
}

// AMState mirrors C AM_State — the breath of the field
type AMState struct {
	// Prophecy physics
	Prophecy      int
	Destiny       float32
	Wormhole      float32
	CalendarDrift float32

	// MetaJanus — the self-location anchor
	BirthSet           bool    // MED-3: true once BIRTH fixed the origin (the born-flag; birth_drift is not injective)
	BirthEpochDays     int     // MED-3: the exact origin day set by BIRTH
	BirthDrift         float32
	PersonalDissonance float32
	JanusGap           float32
	Yahrzeit           float32
	TemporalAlpha      float32 // generic PITOMADOM temporal focus, driven by TEMPORAL_* directives (not by Janus)
	JanusTemporalAlpha float32 // HIGH-2: calendar-derived Janus signal clamp01(0.5+0.5*janus_gap); what D-2 reads when armed

	// Attention
	AttendFocus  float32
	AttendSpread float32

	// Tunneling
	TunnelThreshold float32
	TunnelChance    float32
	TunnelSkipMax   int

	// Suffering
	Pain       float32
	Tension    float32
	Dissonance float32
	Debt       float32

	// Movement
	VelocityMode      int
	VelocityMagnitude float32
	BaseTemperature   float32
	EffectiveTemp     float32
	TimeDirection     float32

	// Wormhole
	WormholeActive int
}

// Pack flags
const (
	PackCodesRIC   = 0x01
	PackDarkMatter = 0x02
	PackNoTorch    = 0x04
)

// Velocity modes
const (
	VelNoMove   = 0
	VelWalk     = 1
	VelRun      = 2
	VelBackward = -1
)

// NewAMK initializes the kernel
func NewAMK() *AMK {
	C.am_init()
	return &AMK{running: true}
}

// Exec executes an AML script
func (a *AMK) Exec(script string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	cs := C.CString(script)
	defer C.free(unsafe.Pointer(cs))

	ret := C.am_exec(cs)
	if ret != 0 {
		return fmt.Errorf("am_exec failed: %d", ret)
	}
	return nil
}

// ExecFile loads and executes an AML script from file
func (a *AMK) ExecFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read AML file: %w", err)
	}

	// Execute line by line (AML is line-oriented)
	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		if err := a.Exec(line); err != nil {
			return fmt.Errorf("AML line %q: %w", line, err)
		}
	}
	return nil
}

// Step advances physics by dt seconds
func (a *AMK) Step(dt float32) {
	a.mu.Lock()
	defer a.mu.Unlock()
	C.am_step(C.float(dt))
}

// GetState reads current kernel state
// JanusKeyArmed reports whether JANUS_KEY is armed (HIGH-1) — distinct from the temporal_alpha value:
// after JANUS_KEY 0 the value freezes at its last pole, but this returns false, so D-2 can de-arm.
func (a *AMK) JanusKeyArmed() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return C.am_janus_key_armed() != 0
}

// CalendarEpochSeconds returns the kernel calendar epoch as absolute UTC seconds (MED-1): the fixed
// 2024-10-03 12:00 UTC = 1727956800, independent of the host timezone/DST. Set by am_init.
func CalendarEpochSeconds() int64 {
	return int64(C.am_calendar_epoch_seconds())
}

func (a *AMK) GetState() AMState {
	a.mu.Lock()
	defer a.mu.Unlock()

	s := C.am_get_state()
	return AMState{
		Prophecy:          int(s.prophecy),
		Destiny:           float32(s.destiny),
		Wormhole:          float32(s.wormhole),
		CalendarDrift:     float32(s.calendar_drift),
		BirthSet:           C.am_birth_set() != 0,
		BirthEpochDays:     int(C.am_birth_epoch_days()),
		BirthDrift:         float32(s.birth_drift),
		PersonalDissonance: float32(s.personal_dissonance),
		JanusGap:           float32(s.janus_gap),
		Yahrzeit:           float32(s.yahrzeit),
		TemporalAlpha:      float32(s.temporal_alpha),
		JanusTemporalAlpha: float32(s.janus_temporal_alpha),
		AttendFocus:       float32(s.attend_focus),
		AttendSpread:      float32(s.attend_spread),
		TunnelThreshold:   float32(s.tunnel_threshold),
		TunnelChance:      float32(s.tunnel_chance),
		TunnelSkipMax:     int(s.tunnel_skip_max),
		Pain:              float32(s.pain),
		Tension:           float32(s.tension),
		Dissonance:        float32(s.dissonance),
		Debt:              float32(s.debt),
		VelocityMode:      int(s.velocity_mode),
		VelocityMagnitude: float32(s.velocity_magnitude),
		BaseTemperature:   float32(s.base_temperature),
		EffectiveTemp:     float32(s.effective_temp),
		TimeDirection:     float32(s.time_direction),
		WormholeActive:    int(s.wormhole_active),
	}
}

// GetTemperature returns AML-modulated temperature
func (a *AMK) GetTemperature() float32 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return float32(C.am_get_temperature())
}

// GetDestinyBias returns destiny bias for sampling
func (a *AMK) GetDestinyBias() float32 {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := C.am_get_state()
	bias := float32(s.destiny_bias)
	if bias == 0 && s.destiny != 0 {
		return float32(s.destiny)
	}
	return bias
}

// ShouldTunnel checks if tunneling should occur
func (a *AMK) ShouldTunnel() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return C.am_should_tunnel() != 0
}

// ApplySufferingToLogits modulates logits by pain/tension
func (a *AMK) ApplySufferingToLogits(logits []float32) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(logits) == 0 {
		return
	}
	C.am_apply_suffering_to_logits((*C.float)(unsafe.Pointer(&logits[0])), C.int(len(logits)))
}

// EnablePack enables an AML extension pack
func (a *AMK) EnablePack(pack uint) {
	a.mu.Lock()
	defer a.mu.Unlock()
	C.am_enable_pack(C.uint(pack))
}

// DisablePack disables an AML extension pack
func (a *AMK) DisablePack(pack uint) {
	a.mu.Lock()
	defer a.mu.Unlock()
	C.am_disable_pack(C.uint(pack))
}

// ResetField resets the field to defaults
func (a *AMK) ResetField() {
	a.mu.Lock()
	defer a.mu.Unlock()
	C.am_reset_field()
}

// ResetDebt resets accumulated debt
func (a *AMK) ResetDebt() {
	a.mu.Lock()
	defer a.mu.Unlock()
	C.am_reset_debt()
}
