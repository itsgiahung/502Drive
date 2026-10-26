package mascot

import "math"

// Mood selects the animation. Callers map daemon state to a mood; the mascot
// never decides what is true.
type Mood string

const (
	Idle  Mood = "idle"
	Busy  Mood = "busy" // files fly from the source folder over the cat to the destination
	Happy Mood = "happy"
	Error Mood = "error"
	Wait  Mood = "wait" // amber: the daemon needs the user (reconnect, blocked items)
	Sleep Mood = "sleep"
)

// Frame is one drawn picture.
type Frame struct{ cv *canvas }

type spring struct{ pos, vel float64 }

// step moves the spring toward target with a slightly bouncy, stable
// integration (four sub-steps so a slow tick cannot blow it up).
func (s *spring) step(target, dt float64) float64 {
	const k, d = 260.0, 2 * 16.1 * .62
	for i := 0; i < 4; i++ {
		h := dt / 4
		s.vel += (k*(target-s.pos) - d*s.vel) * h
		s.pos += s.vel * h
	}
	return s.pos
}

// Anim keeps the spring state so a mood change eases instead of snapping.
type Anim struct {
	sx, sy, lift, shake, ear, droop spring
	mood                            Mood
	t                               float64
}

func NewAnim() *Anim {
	a := &Anim{mood: Idle}
	a.sx.pos, a.sy.pos = S, S
	return a
}

// Step advances the animation by dt seconds in mood m and draws it.
func (a *Anim) Step(m Mood, dt float64) *Frame {
	if m != a.mood {
		a.mood, a.t = m, 0
	}
	dt = math.Min(dt, .25)
	a.t += dt
	tp, f := target(string(m), a.t)
	p := pose{
		sx: a.sx.step(tp.sx, dt), sy: a.sy.step(tp.sy, dt), lift: a.lift.step(tp.lift, dt),
		shake: a.shake.step(tp.shake, dt), ear: a.ear.step(tp.ear, dt), droop: a.droop.step(tp.droop, dt),
	}
	return &Frame{scene(string(m), a.t, p, f)}
}

// Static draws mood m at time t without spring smoothing (single prints,
// tests).
func Static(m Mood, t float64) *Frame {
	p, f := target(string(m), t)
	return &Frame{scene(string(m), t, p, f)}
}
