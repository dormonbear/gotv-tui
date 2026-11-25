// floorgen learns each map's ground and storey heights from CS2 demos, for
// internal/radar/floors.json: go run ./scripts/floorgen demo.dem... > internal/radar/floors.json
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"

	demoinfocs "github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/events"
	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/msg"
)

const cell = 100.0 // must match radar.floorCell

func main() {
	out := map[string]map[[2]int][2]float64{} // map -> cell -> lo, hi
	for _, path := range os.Args[1:] {
		f, err := os.Open(path)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		p := demoinfocs.NewParser(f)
		mapName := ""
		p.RegisterNetMessageHandler(func(m *msg.CSVCMsg_ServerInfo) { mapName = m.GetMapName() })
		p.RegisterEventHandler(func(events.FrameDone) {
			if mapName == "" {
				return
			}
			if out[mapName] == nil {
				out[mapName] = map[[2]int][2]float64{}
			}
			for _, pl := range p.GameState().Participants().Playing() {
				if !pl.IsAlive() {
					continue
				}
				v := pl.Position()
				k := [2]int{int(math.Floor(v.X / cell)), int(math.Floor(v.Y / cell))}
				s, ok := out[mapName][k]
				if !ok {
					s = [2]float64{v.Z, v.Z}
				}
				out[mapName][k] = [2]float64{math.Min(s[0], v.Z), math.Max(s[1], v.Z)}
			}
		})
		err = p.ParseToEnd(context.Background())
		f.Close()
		fmt.Fprintf(os.Stderr, "%s: %s (%v)\n", path, mapName, err)
	}
	// compact: map -> [[kx, ky, lo, hi], ...], heights rounded
	res := map[string][][4]int{}
	for m, cells := range out {
		for k, s := range cells {
			res[m] = append(res[m], [4]int{k[0], k[1], int(math.Round(s[0])), int(math.Round(s[1]))})
		}
		sort.Slice(res[m], func(i, j int) bool {
			a, b := res[m][i], res[m][j]
			return a[0] < b[0] || a[0] == b[0] && a[1] < b[1]
		})
	}
	json.NewEncoder(os.Stdout).Encode(res)
}
