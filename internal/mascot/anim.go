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
type Frame struct {
	cv     *canvas
	p      pose
	face   face
	mood   Mood
	t      float64
	accent rgb
}

type spring struct{ pos, vel float64 }

// step solves the damped spring exactly for a constant target.
func (s *spring) step(target, dt float64) float64 {
	if dt <= 0 || math.IsNaN(dt) || math.IsInf(dt, 0) {
		return s.pos
	}
	const decay, omega = 11.6, 11.2
	e := math.Exp(-decay * dt)
	x := s.pos - target
	v := (s.vel + decay*x) / omega
	sn, cs := math.Sincos(omega * math.Min(dt, 60))
	s.pos = target + e*(x*cs+v*sn)
	s.vel = e * ((v*omega-decay*x)*cs - (x*omega+decay*v)*sn)
	return s.pos
}

// Anim keeps the spring state and reuses its drawing workspace. Returned Frames
// own their snapshots, so retaining a frame never observes a subsequent Step.
type Anim struct {
	sx, sy, lift, shake, ear, droop     spring
	tilt, head, tail, paw, curl, fright spring
	mood                                Mood
	t, clock, scale                     float64
	props                               bool
	// Seed controls deterministic gaze, fidgets and irregular blinks.
	// Set it before the first Step; the default is the same for every animation.
	Seed   uint64
	face   face
	accent rgb
	cv     canvas
}

func NewAnim() *Anim { return newAnim(S, true) }

// NewMini is the panel cat without folders or flying files.
func NewMini() *Anim { return newAnim(MiniScale, false) }

func newAnim(scale float64, props bool) *Anim {
	a := &Anim{mood: Idle, scale: scale, props: props, Seed: motionSeed, accent: cBlush}
	a.sx.pos, a.sy.pos = scale*.82, scale
	_, a.face = target(Idle, 0, 0, scale, a.Seed)
	return a
}

// Step advances all motion by elapsed seconds. Invalid/negative dt freezes time;
// after a long pause only the final quarter second needs spring integration.
func (a *Anim) Step(m Mood, dt float64) *Frame {
	if m != a.mood {
		a.mood, a.t = m, 0
	}
	if dt < 0 || math.IsNaN(dt) || math.IsInf(dt, 0) {
		dt = 0
	}
	dt = min(dt, 60)
	skip := max(0, dt-.25)
	a.t, a.clock = a.t+skip, sceneTime(a.clock+skip)
	dt -= skip
	n := max(1, int(math.Ceil(dt*120)))
	h := dt / float64(n)
	var p pose
	for range n {
		a.t, a.clock = sceneTime(a.t+h), sceneTime(a.clock+h)
		tp, f := target(m, a.t, a.clock, a.scale, a.Seed)
		p = pose{
			sx: a.sx.step(tp.sx, h), sy: a.sy.step(tp.sy, h), lift: a.lift.step(tp.lift, h),
			shake: a.shake.step(tp.shake, h), ear: a.ear.step(tp.ear, h), droop: a.droop.step(tp.droop, h),
			tilt: a.tilt.step(tp.tilt, h), head: a.head.step(tp.head, h), tail: a.tail.step(tp.tail, h),
			paw: a.paw.step(tp.paw, h), curl: a.curl.step(tp.curl, h), fright: a.fright.step(tp.fright, h),
		}
		k, blink := 1-math.Exp(-h/0.10), 1-math.Exp(-h/0.018)
		a.face.gx += (f.gx - a.face.gx) * k
		a.face.gy += (f.gy - a.face.gy) * k
		a.face.pupil += (f.pupil - a.face.pupil) * k
		a.face.blink += (f.blink - a.face.blink) * blink
		a.face.joy += (f.joy - a.face.joy) * k
		a.face.open += (f.open - a.face.open) * k
		a.face.smile += (f.smile - a.face.smile) * k
		a.face.blush += (f.blush - a.face.blush) * k
		a.face.think += (f.think - a.face.think) * k
		a.accent = mix(a.accent, moodAccent(m), k)
	}
	scene(&a.cv, m, a.clock, p, a.face, a.props, a.accent)
	snapshot := a.cv
	return &Frame{cv: &snapshot, p: p, face: a.face, mood: m, t: a.clock, accent: a.accent}
}

// Static draws mood m at time t without spring smoothing (single prints, tests).
func Static(m Mood, t float64) *Frame { return static(m, t, S, true) }

// StaticMini is Static for the panel cat.
func StaticMini(m Mood, t float64) *Frame { return static(m, t, MiniScale, false) }

func static(m Mood, t, scale float64, props bool) *Frame {
	t = sceneTime(t)
	p, f := target(m, t, t, scale, motionSeed)
	cv := &canvas{}
	scene(cv, m, t, p, f, props, moodAccent(m))
	return &Frame{cv: cv, p: p, face: f, mood: m, t: t, accent: moodAccent(m)}
}
