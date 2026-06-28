package yent

import "testing"

func TestLimphaStateFromAMState(t *testing.T) {
	st := LimphaStateFromAMState(AMState{
		EffectiveTemp: 1.2,
		Destiny:       0.35,
		Pain:          0.1,
		Tension:       0.2,
		Debt:          0.3,
		VelocityMode:  VelRun,
	}, 0.45)

	if st.Temperature != 1.2 || st.Destiny != 0.35 || st.Pain != 0.1 ||
		st.Tension != 0.2 || st.Debt != 0.3 || st.Velocity != VelRun ||
		st.Alpha != 0.45 {
		t.Fatalf("unexpected limpha state: %+v", st)
	}
}
