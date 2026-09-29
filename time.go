package illusion

import "time"

// maxDelta caps a single frame's delta so a stall (a breakpoint, a window
// drag) doesn't make the fixed loop run hundreds of steps to catch up.
const maxDelta = 250 * time.Millisecond

// Time is the clock systems read. In the Fixed* schedules it reports the fixed
// timestep instead of the frame delta, so the same system works in both.
type Time struct {
	delta   time.Duration
	elapsed time.Duration
	frame   uint64
	fixed   bool
}

// Delta is the time since the previous frame (or fixed step).
func (t *Time) Delta() time.Duration { return t.delta }

// DeltaSecs is Delta in seconds.
func (t *Time) DeltaSecs() float32 { return float32(t.delta.Seconds()) }

// Elapsed is the total time since startup.
func (t *Time) Elapsed() time.Duration { return t.elapsed }

// ElapsedSecs is Elapsed in seconds.
func (t *Time) ElapsedSecs() float32 { return float32(t.elapsed.Seconds()) }

// Frame is the number of frames completed.
func (t *Time) Frame() uint64 { return t.frame }

// InFixedStep reports whether the Fixed* schedules are running, i.e. whether
// Delta is the fixed timestep.
func (t *Time) InFixedStep() bool { return t.fixed }

// FixedTime controls the Fixed* schedules.
type FixedTime struct {
	// Timestep is how much time each fixed step simulates. Defaults to 1/60s.
	Timestep time.Duration

	elapsed     time.Duration
	accumulated time.Duration
}

// Overstep is the fraction of a timestep accumulated but not yet simulated,
// in [0, 1). Use it to interpolate rendering between fixed steps.
func (f *FixedTime) Overstep() float32 {
	if f.Timestep <= 0 {
		return 0
	}
	return float32(f.accumulated) / float32(f.Timestep)
}
