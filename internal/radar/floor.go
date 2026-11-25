package radar

import (
	"math"

	"github.com/dormonbear/gotv-tui/internal/gotv"
)

// Floor learns the ground height of the map as players walk it: the lowest z
// seen in each floorCell-sized square. A player well above that (on a box, a
// ledge, or mid-jump) is drawn with a ▴.
// ponytail: learnt, not read from map geometry; a spot nobody has stood on below
// yet has no floor, so the marker appears once the ground there has been walked.
type Floor map[[2]int]float64

const (
	floorCell = 100.0 // world units per square
	raisedBy  = 56.0  // ~ a crate; less is stairs and slopes
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
		k := floorKey(*p.Pos)
		if z, ok := f[k]; !ok || p.Pos.Z < z {
			f[k] = p.Pos.Z
		}
	}
}

// Raised reports whether v is clearly above the ground learnt at that spot.
func (f Floor) Raised(v gotv.Vec) bool {
	z, ok := f[floorKey(v)]
	return ok && v.Z-z > raisedBy
}
