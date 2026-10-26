package mascot

import "math"

const motionSeed uint64 = 502

// hash samples a fixed seed and time bucket, never the number of drawn frames.
func noise(seed uint64, bucket int64) float64 {
	x := seed + uint64(bucket)*0x9e3779b97f4a7c15
	x = (x ^ (x >> 30)) * 0xbf58476d1ce4e5b9
	x = (x ^ (x >> 27)) * 0x94d049bb133111eb
	return float64((x^(x>>31))>>11) / (1 << 53)
}

func frac(x float64) float64 { return x - math.Floor(x) }

// Bound procedural phases before converting them to integer sprite indices.
func sceneTime(t float64) float64 {
	if math.IsNaN(t) || math.IsInf(t, 0) {
		return 0
	}
	return math.Mod(math.Max(0, t), 86400)
}

func smooth(x float64) float64 {
	x = max(0, min(1, x))
	return x * x * (3 - 2*x)
}

func pulse(t, start, length float64) float64 {
	x := (t - start) / length
	if x <= 0 || x >= 1 {
		return 0
	}
	return math.Pow(math.Sin(x*math.Pi), 2)
}

func blinkAt(t float64, seed uint64) float64 {
	bucket := int64(t / 4)
	local := math.Mod(t, 4)
	start := 1.1 + noise(seed, bucket)*1.9
	length := .18 + .20*noise(seed+1, bucket)
	b := pulse(local, start, length)
	if noise(seed+2, bucket) > .65 {
		b = max(b, pulse(local, start+length+.12, .19))
	}
	return b
}

// trackPacket points at the same phase used by the separate transfer strip.
func trackPacket(t float64) (gx, gy float64) {
	best, bd := 0.0, 9.0
	for _, ph := range flow(t * 1.3) {
		if d := math.Abs(ph - .5); d < bd {
			best, bd = ph, d
		}
	}
	gx = (best - .5) * 5
	gy = -1.3 * math.Sin(best*math.Pi)
	return
}

func target(mood Mood, t, clock, sc float64, seed uint64) (pose, face) {
	breath := math.Sin(2 * clock)
	p := pose{sx: sc * .82 * (1 - .012*breath), sy: sc * (1 + .018*breath)}
	p.tail = smooth(clock/1.4) * (.7 + .18*math.Sin(clock*1.6-.7))
	p.ear = .5 * math.Sin(clock*2-.8) * smooth(clock)
	f := face{pupil: .25, smile: 1, blush: .45, blink: blinkAt(clock, seed)}
	bucket := int64(clock / 2.3)
	gaze := (noise(seed+8, bucket)*2 - 1) * 1.4 * smooth(clock/2)
	f.gx, f.gy = gaze, .3*math.Sin(clock*.8)
	p.tilt, p.head = gaze*.035, .15*math.Sin(clock*2-.5)
	switch mood {
	case Idle:
		// One fidget per twelve seconds; breathing and gaze continue underneath.
		bucket := int64(clock / 12)
		local := math.Mod(clock, 12)
		u := pulse(local, 6.5+noise(seed+3, bucket), 2.4)
		switch int(noise(seed+4, bucket) * 5) {
		case 0: // yawn
			f.open, f.blink, p.droop = u, max(f.blink, u*.8), 2*u
		case 1: // stretch: compress, then reach up
			p.sy *= 1 + .10*u
			p.sx *= 1 - .05*u
			p.paw, p.head = .8*u, -.9*u
		case 2: // wash a cheek
			p.paw, p.tilt = 1.8*u, -.12*u
			f.blink = max(f.blink, .7*u)
		case 3: // glance toward the input below, then across the room
			f.gx, f.gy = gaze+2*u*math.Sin(local*2), 1.3*u
			p.ear += u
		case 4: // settle into a lower sit
			p.sy *= 1 - .08*u
			p.droop, f.blink = u, max(f.blink, .6*u)
		}
	case Busy:
		f.gx, f.gy = trackPacket(clock)
		f.pupil, f.smile, f.blush = .55, .2, .2
		p.tilt = f.gx * .085
		p.head = .3 * math.Sin(clock*4-1)
		p.paw = .55 * math.Sin(clock*7)
		p.ear += f.gx * .5
		p.tail *= .7
	case Happy:
		// Crouch before takeoff, stretch in flight, then settle on landing.
		ph := frac(t / 1.35)
		crouch := pulse(ph, 0, .23)
		jump := pulse(ph, .23, .65)
		land := pulse(ph, .88, .12)
		p.lift = 1.0 * jump
		p.head -= .7 * jump // the head lags takeoff, preserving room above the ears
		p.sx *= 1 + .07*crouch - .04*jump + .05*land
		p.sy *= 1 - .09*crouch + .035*jump - .07*land
		p.tilt = .08 * math.Sin(t*4)
		p.ear += .9 * math.Sin(t*5-.5)
		p.paw, f.joy, f.open, f.blush, f.pupil = .4*jump, 1, .85, 1, 1
		f.blink = 0
	case Error:
		a := math.Exp(-t * 1.2)
		p.fright = a
		p.shake = .75 * math.Sin(t*32) * a
		p.sy *= 1 + .035*a
		p.ear, p.droop = 2.5*a, 3*(1-a)
		f.pupil, f.smile, f.think = 1-.6*(1-a), -.7, .75
		f.gx *= .3
	case Wait:
		p.tilt = .13 + .025*math.Sin(clock*1.3)
		p.droop, p.ear = 1.2, .5
		f.gx, f.gy, f.smile, f.think = -.7, -.8, .1, .45
	case Sleep:
		s := math.Sin(clock * 1.1)
		p.curl, p.tilt, p.droop = 1, -.12, 3
		p.sy, p.sx = sc*(.91+.025*s), sc*.86
		p.tail = .32 + .04*math.Sin(clock*1.1-.8)
		p.head = -.2 + .12*math.Sin(clock*1.1-.5)
		f.blink, f.open, f.smile, f.blush = 1, 0, .6, .5
	}
	return p, f
}

func moodAccent(m Mood) rgb {
	switch m {
	case Busy:
		return cGold
	case Happy:
		return hx(130, 235, 190)
	case Error:
		return cCoral
	case Wait:
		return hx(255, 184, 112)
	case Sleep:
		return hx(177, 157, 235)
	default:
		return cBlush
	}
}
