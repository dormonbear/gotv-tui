// Package director picks what the radar camera should frame, like an esports observer.
//
// Modelled on Valve's HLTV auto-director (source-sdk-2013 hltvdirector.cpp): events
// carry priorities and are read a few seconds before they play, so the camera arrives
// before the kill; a shot lasts 4-8 s; without events, players are ranked by enemies
// nearby (sum of 1/sqrt(distance)). Shots are picked and then tracked rather than
// re-picked every frame, the fix for jumpy cameras in automatic observers.
package director

import (
	"math"
	"strings"
	"time"

	"github.com/dormonbear/gotv-tui/internal/gotv"
	"github.com/dormonbear/gotv-tui/internal/playback"
)

const (
	minShot   = 4 * time.Second
	maxShot   = 8 * time.Second
	cutMargin = 3.0             // an early cut needs this much more priority than the current shot
	quiet     = 1.5             // below this nothing is worth framing: show the whole map
	nearby    = 1500            // world units: enemies closer than this make a fight
	margin    = 450             // world units of context around the framed players
	minRadius = 700             // never zoom in tighter than this half-size
	heatHalf  = 2.0             // seconds for damage heat to halve
	wideHold  = 2 * time.Second // after pulling out to the whole map, stay there at least this long
)

// Priorities on Valve's 0..10 scale.
const (
	prioKill    = 6.0
	prioClutch  = 2.0 // extra for a kill when the victim's side is down to two or fewer
	prioPlant   = 8.0
	prioDefuse  = 9.0
	prioPlanted = 4.0 // post-plant: the bomb is where the round is decided
)

// Shot is what the camera should frame, in world units.
type Shot struct {
	X, Y   float64
	Radius float64 // half-size of the area to keep in view
	Reason string
}

type hotspot struct {
	prio    float64
	members []string // players to keep in frame (lowercase names)
	extra   []gotv.Vec
	reason  string
}

type Director struct {
	cur     *hotspot
	wideAt  time.Time // when the camera last pulled out to the whole map
	since   time.Time
	heat    map[string]float64 // recent damage taken, decaying
	lastHP  map[string]int
	lastAt  time.Time
	lastPos map[string]gotv.Vec
}

func New() *Director {
	return &Director{heat: map[string]float64{}, lastHP: map[string]int{}, lastPos: map[string]gotv.Vec{}}
}

func key(n string) string { return strings.ToLower(n) }

func dist(a, b gotv.Vec) float64 { return math.Hypot(a.X-b.X, a.Y-b.Y) }

// Update takes the state on screen and what is queued ahead, and returns the shot to
// frame, or nil for the whole map.
func (d *Director) Update(now time.Time, st *gotv.State, soon []playback.Soon[gotv.Event], lowerBelow *float64) *Shot {
	if st == nil || st.Freeze || st.Over {
		d.cur = nil
		return nil
	}
	d.track(now, st)
	best := d.best(st, soon, lowerBelow)

	switch {
	case d.cur == nil,
		now.Sub(d.since) >= maxShot,
		now.Sub(d.since) >= minShot && (best == nil || best.prio > d.cur.prio || d.score(st, d.cur, lowerBelow) < quiet),
		best != nil && best.prio >= d.cur.prio+cutMargin:
		if best == nil || best.prio < quiet {
			if d.cur != nil {
				d.wideAt = now
			}
			d.cur = nil
			return nil
		}
		// just pulled out: a passing skirmish doesn't yank it back in, an event does
		if d.cur == nil && now.Sub(d.wideAt) < wideHold && len(best.extra) == 0 {
			return nil
		}
		if d.cur == nil || !sameFight(d.cur, best) {
			d.since = now
		}
		d.cur = best
	}
	return d.frame(st)
}

// track updates damage heat and last known positions.
func (d *Director) track(now time.Time, st *gotv.State) {
	if !d.lastAt.IsZero() {
		k := math.Pow(0.5, now.Sub(d.lastAt).Seconds()/heatHalf)
		for n := range d.heat {
			d.heat[n] *= k
		}
	}
	d.lastAt = now
	for _, p := range st.Players {
		n := key(p.Name)
		if hp, ok := d.lastHP[n]; ok && p.HP < hp {
			d.heat[n] += float64(hp - p.HP)
		}
		d.lastHP[n] = p.HP
		if p.Pos != nil {
			d.lastPos[n] = *p.Pos
		}
	}
}

func sameLevel(a, b gotv.Vec, lowerBelow *float64) bool {
	return lowerBelow == nil || (a.Z < *lowerBelow) == (b.Z < *lowerBelow)
}

// fight scores alive player p by enemies nearby (Valve's 1/sqrt(dist)) plus recent damage.
func (d *Director) fight(st *gotv.State, p gotv.Player, lowerBelow *float64) (float64, []string) {
	if !p.Alive || p.Pos == nil {
		return 0, nil
	}
	score := d.heat[key(p.Name)] / 20 // 100 damage ~ 5
	members := []string{key(p.Name)}
	for _, e := range st.Players {
		if !e.Alive || e.Pos == nil || e.Side == p.Side || !sameLevel(*p.Pos, *e.Pos, lowerBelow) {
			continue
		}
		if dd := dist(*p.Pos, *e.Pos); dd < nearby {
			score += 30 / math.Sqrt(max(dd, 64)) // 400u ~ 1.5, 900u ~ 1.0
			members = append(members, key(e.Name))
		}
	}
	if len(members) == 1 {
		score = d.heat[key(p.Name)] / 40 // took damage but no enemy near: probably utility
	}
	return score, members
}

func (d *Director) best(st *gotv.State, soon []playback.Soon[gotv.Event], lowerBelow *float64) *hotspot {
	var best *hotspot
	consider := func(h *hotspot) {
		if best == nil || h.prio > best.prio {
			best = h
		}
	}
	for _, p := range st.Players {
		if s, m := d.fight(st, p, lowerBelow); s > 0 {
			consider(&hotspot{prio: s, members: m, reason: fightName(st, m)})
		}
	}
	if b := st.Bomb; b != nil && b.State != gotv.BombDropped {
		h := &hotspot{prio: prioPlanted, extra: []gotv.Vec{b.Pos}, reason: "bomb"}
		if b.State == gotv.BombDefusing {
			h.prio, h.reason = prioDefuse, "defuse"
		}
		for _, p := range st.Players { // whoever is near the bomb is in the shot
			if p.Alive && p.Pos != nil && dist(*p.Pos, b.Pos) < nearby {
				h.members = append(h.members, key(p.Name))
			}
		}
		consider(h)
	}
	alive := map[gotv.Side]int{}
	for _, p := range st.Players {
		if p.Alive {
			alive[p.Side]++
		}
	}
	for _, s := range soon { // read ahead: be there before it happens
		e := s.E
		switch e.Kind {
		case gotv.EvKill:
			if e.Killer == nil || e.Victim == nil || e.Killer.Name == e.Victim.Name {
				continue
			}
			prio := prioKill
			if alive[e.Victim.Side] <= 2 {
				prio += prioClutch
			}
			h := &hotspot{prio: prio, members: []string{key(e.Killer.Name), key(e.Victim.Name)}, reason: e.Killer.Name + " → " + e.Victim.Name}
			if e.To != nil {
				h.extra = append(h.extra, *e.To)
			}
			consider(h)
		case gotv.EvPlant:
			consider(&hotspot{prio: prioPlant, members: []string{key(e.Player)}, reason: "plant"})
		case gotv.EvDefuse:
			consider(&hotspot{prio: prioDefuse, members: []string{key(e.Player)}, reason: "defuse"})
		}
	}
	return best
}

// score re-rates the current hotspot on the latest state, so a finished fight ends its shot.
func (d *Director) score(st *gotv.State, h *hotspot, lowerBelow *float64) float64 {
	if len(h.extra) > 0 { // event or bomb shots run their course
		return h.prio
	}
	top := 0.0
	for _, p := range st.Players {
		for _, m := range h.members {
			if key(p.Name) == m {
				s, _ := d.fight(st, p, lowerBelow)
				top = max(top, s)
			}
		}
	}
	return top
}

func sameFight(a, b *hotspot) bool {
	for _, x := range a.members {
		for _, y := range b.members {
			if x == y {
				return true
			}
		}
	}
	return false
}

func fightName(st *gotv.State, members []string) string {
	if len(members) < 2 {
		return "fight"
	}
	name := func(k string) string {
		for _, p := range st.Players {
			if key(p.Name) == k {
				return p.Name
			}
		}
		return k
	}
	return name(members[0]) + " vs " + name(members[1])
}

// frame fits the current hotspot's players (where they are now) and fixed points.
func (d *Director) frame(st *gotv.State) *Shot {
	pts := append([]gotv.Vec(nil), d.cur.extra...)
	for _, m := range d.cur.members {
		if p, ok := d.lastPos[m]; ok {
			pts = append(pts, p)
		}
	}
	if len(pts) == 0 {
		return nil
	}
	x0, y0, x1, y1 := pts[0].X, pts[0].Y, pts[0].X, pts[0].Y
	for _, p := range pts[1:] {
		x0, y0, x1, y1 = min(x0, p.X), min(y0, p.Y), max(x1, p.X), max(y1, p.Y)
	}
	r := max(minRadius, max(x1-x0, y1-y0)/2+margin)
	return &Shot{X: (x0 + x1) / 2, Y: (y0 + y1) / 2, Radius: r, Reason: d.cur.reason}
}
