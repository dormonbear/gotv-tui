package radar

import (
	_ "embed"
	"encoding/json"
	"math"

	"github.com/dormonbear/gotv-tui/internal/gotv"
)

// Floor learns the heights players stand at in each floorCell-sized square: the
// lowest is the ground. A player well above it (on a box, a ledge, mid-jump) is
// drawn with a ▴; one far below the highest (under a bridge or a floor someone
// has walked above) with a ▾.
// ponytail: learnt, not read from map geometry; a spot nobody has stood on below
// yet has no floor, so the marker appears once the ground there has been walked.
type Floor map[[2]int]span

type span struct{ lo, hi float64 }

const (
	floorCell = 100.0 // world units per square
	raisedBy  = 56.0  // ~ a crate; less is stairs and slopes
	underBy   = 150.0 // a storey: well over a crate plus a jump
)

func floorKey(v gotv.Vec) [2]int {
	return [2]int{int(math.Floor(v.X / floorCell)), int(math.Floor(v.Y / floorCell))}
}

// Observe records where the living players stand.
func (f Floor) Observe(ps []gotv.Player) {
	for _, p := range ps {
		if !p.Alive || p.Pos == nil {
			continue
		}
		k, z := floorKey(*p.Pos), p.Pos.Z
		if s, ok := f[k]; ok {
			f[k] = span{min(s.lo, z), max(s.hi, z)}
		} else {
			f[k] = span{z, z}
		}
	}
}

// Raised reports whether v is clearly above the ground learnt at that spot.
func (f Floor) Raised(v gotv.Vec) bool {
	s, ok := f[floorKey(v)]
	return ok && v.Z-s.lo > raisedBy
}

// Under reports whether someone has stood a storey above v.
func (f Floor) Under(v gotv.Vec) bool {
	s, ok := f[floorKey(v)]
	return ok && s.hi-v.Z > underBy
}

//go:embed floors.json
var floorsJSON []byte

// KnownFloor is the floor learnt ahead of time from demos (scripts/floorgen), so
// the markers work from the first second; live play keeps adding to it. Empty
// for maps without data.
func KnownFloor(mapName string) Floor {
	var all map[string][][4]int
	f := Floor{}
	if json.Unmarshal(floorsJSON, &all) != nil {
		return f
	}
	for _, c := range all[mapName] {
		f[[2]int{c[0], c[1]}] = span{float64(c[2]), float64(c[3])}
	}
	return f
}
