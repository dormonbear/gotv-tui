package audio

import "image"

// tickDetector finds the frame on which the clock changes, by pixel change in the
// clock's own box. Ported from Fy1Sync's TickMomentDetector (GPL-3.0): a pixel
// counts as changed when it moves by 24+ levels and is bright on either side; a
// tick is a jump in the changed share well above the running noise floor, with
// hysteresis and a 0.3 s refractory time.
type tickDetector struct {
	prev     []byte
	prevBox  box
	prevPts  float64
	hasPrev  bool
	lastTick float64
	hasTick  bool
	noise    float64
	high     bool
}

// push looks at the clock box b of img at stream time pts; it returns the moment
// of a tick (midway between the last unchanged frame and this one).
func (d *tickDetector) push(img *image.Gray, b box, pts float64) (float64, bool) {
	cur := make([]byte, 0, b.w()*b.h())
	for y := b.y0; y <= b.y1; y++ {
		cur = append(cur, img.Pix[y*img.Stride+b.x0:y*img.Stride+b.x1+1]...)
	}
	defer func() { d.prev, d.prevBox, d.prevPts, d.hasPrev = cur, b, pts, true }()
	if !d.hasPrev || d.prevBox != b || pts <= d.prevPts || len(cur) == 0 {
		d.high = false
		return 0, false
	}
	changed := 0
	for i, v := range cur {
		a := d.prev[i]
		diff := int(a) - int(v)
		if (diff >= 24 || diff <= -24) && (a >= 145 || v >= 145) {
			changed++
		}
	}
	share := float64(changed) / float64(len(cur))
	threshold := max(d.noise*5+0.008, 0.008)
	if share < threshold*0.45 {
		d.high = false
	}
	refractory := d.hasTick && pts-d.lastTick < 0.30
	switch {
	case share >= threshold && !d.high && !refractory:
		d.high, d.lastTick, d.hasTick = true, pts, true
		return (d.prevPts + pts) / 2, true
	case share < threshold && !d.high:
		d.noise = d.noise*0.94 + share*0.06
	}
	return 0, false
}

// box is the clock's box on screen (digits and colon, a little margin).
func (l *layout) box(img *image.Gray) box {
	return box{max(0, l.digits[0]-l.pitch), l.y0, min(img.Rect.Dx()-1, l.digits[len(l.digits)-1]+l.pitch), l.y1}
}
