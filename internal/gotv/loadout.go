package gotv

import (
	"sort"

	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/common"
)

var nadeOf = map[common.EquipmentType]Nade{
	common.EqHE:         HE,
	common.EqFlash:      Flash,
	common.EqSmoke:      Smoke,
	common.EqMolotov:    Fire,
	common.EqIncendiary: Fire,
	common.EqDecoy:      Decoy,
}

var nadeOrder = map[Nade]int{HE: 0, Flash: 1, Smoke: 2, Fire: 3, Decoy: 4}

// Item is the part of a carried weapon the loadout needs.
type Item struct {
	Type  common.EquipmentType
	Count int // only flashbangs stack (2)
}

// Loadout picks the primary gun (else the pistol) and lists grenades in a fixed order.
func Loadout(items []Item) (gun string, nades []Nade, hasBomb bool) {
	var pistol string
	for _, it := range items {
		switch c := it.Type.Class(); {
		case it.Type == common.EqBomb:
			hasBomb = true
		case c == common.EqClassGrenade:
			if n, ok := nadeOf[it.Type]; ok {
				for i := 0; i < max(1, it.Count); i++ {
					nades = append(nades, n)
				}
			}
		case c == common.EqClassPistols:
			pistol = it.Type.String()
		case c == common.EqClassSMG || c == common.EqClassHeavy || c == common.EqClassRifle:
			gun = it.Type.String()
		}
	}
	if gun == "" {
		gun = pistol
	}
	sort.SliceStable(nades, func(i, j int) bool { return nadeOrder[nades[i]] < nadeOrder[nades[j]] })
	return gun, nades, hasBomb
}
