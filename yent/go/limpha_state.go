package yent

// LimphaStateFromAMState converts the live AML/AMK field into the compact state
// vector stored by limpha and carried through the two-body router trace.
func LimphaStateFromAMState(s AMState, alpha float32) LimphaState {
	return LimphaState{
		Temperature: s.EffectiveTemp,
		Destiny:     s.Destiny,
		Pain:        s.Pain,
		Tension:     s.Tension,
		Debt:        s.Debt,
		Velocity:    s.VelocityMode,
		Alpha:       alpha,
	}
}
