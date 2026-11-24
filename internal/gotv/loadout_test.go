package gotv

import (
	"reflect"
	"testing"

	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/common"
)

func TestLoadout(t *testing.T) {
	gun, nades, bomb := Loadout([]Item{
		{Type: common.EqKnife}, {Type: common.EqGlock}, {Type: common.EqAK47},
		{Type: common.EqFlash, Count: 2}, {Type: common.EqHE}, {Type: common.EqBomb},
	})
	if gun != "AK-47" || !reflect.DeepEqual(nades, []Nade{HE, Flash, Flash}) || !bomb {
		t.Fatalf("got %q %v %v", gun, nades, bomb)
	}
	gun, nades, bomb = Loadout([]Item{{Type: common.EqKnife}, {Type: common.EqDeagle}, {Type: common.EqIncendiary}, {Type: common.EqSmoke}})
	if gun != "Desert Eagle" || !reflect.DeepEqual(nades, []Nade{Smoke, Fire}) || bomb {
		t.Fatalf("got %q %v %v", gun, nades, bomb)
	}
	if gun, nades, _ := Loadout([]Item{{Type: common.EqKnife}}); gun != "" || nades != nil {
		t.Fatalf("knife only: %q %v", gun, nades)
	}
}
