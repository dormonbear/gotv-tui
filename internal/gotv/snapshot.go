package gotv

import (
	"fmt"

	demoinfocs "github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/common"
	st "github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/sendtables"
)

var projectileKind = map[common.EquipmentType]UtilityKind{
	common.EqHE:         UHE,
	common.EqFlash:      UFlash,
	common.EqMolotov:    UMolotov,
	common.EqIncendiary: UMolotov,
	common.EqDecoy:      UDecoy,
	common.EqSmoke:      USmokeNade,
}

func prop(e *st.Entity, name string) (st.PropertyValue, bool) {
	if e == nil {
		return st.PropertyValue{}, false
	}
	v, ok := e.PropertyValue(name)
	return v, ok && v.Any != nil
}

// adr: some servers leave m_matchStats damage at 0; per-round stats are always filled.
func adr(pl *common.Player, rounds int) float64 {
	sum := 0
	for i := 0; i < rounds; i++ {
		if v, ok := prop(pl.Entity, fmt.Sprintf("m_pActionTrackingServices.m_perRoundStats.%04d.m_iDamage", i)); ok {
			sum += v.Int()
		}
	}
	return float64(max(sum, pl.TotalDamage())) / float64(max(1, rounds))
}

func snapshot(p *demoinfocs.Parser, mapName string, smokes map[int]Vec) State {
	gs := p.GameState()
	now := p.CurrentTime().Seconds()
	played := gs.TotalRoundsPlayed()
	team := func(ts *common.TeamState, fallback string) Team {
		if ts == nil {
			return Team{Name: fallback}
		}
		n := clean(ts.ClanName())
		if n == "" {
			n = fallback
		}
		return Team{Name: n, Score: ts.Score()}
	}
	s := State{
		Map:         mapName,
		CT:          team(gs.TeamCounterTerrorists(), "CT"),
		T:           team(gs.TeamTerrorists(), "T"),
		Round:       played + 1,
		Freeze:      gs.IsFreezetimePeriod(),
		Warmup:      gs.IsWarmupPeriod(),
		Over:        gs.GamePhase() == common.GamePhaseGameEnded || gs.GamePhase() == common.GamePhaseGameOver,
		SecondsLeft: -1,
	}

	for _, pl := range gs.Participants().Playing() {
		side := sideOf(pl.Team)
		if side == "" {
			continue
		}
		items := make([]Item, 0, len(pl.Weapons()))
		for _, w := range pl.Weapons() {
			n := 1
			if w.Type == common.EqFlash {
				n = int(pl.FlashbangCount())
			}
			items = append(items, Item{Type: w.Type, Count: n})
		}
		gun, nades, bomb := Loadout(items)
		var pos *Vec
		if pl.Entity != nil {
			pos = vec(pl.Position())
		}
		s.Players = append(s.Players, Player{
			Name: clean(pl.Name), Side: side, Pos: pos, Yaw: float64(pl.ViewDirectionX()),
			Alive: pl.IsAlive(), HP: pl.Health(), Armor: pl.Armor(), Helmet: pl.HasHelmet(), Defuser: pl.HasDefuseKit(),
			Money: pl.Money(), Kills: pl.Kills(), Deaths: pl.Deaths(), Assists: pl.Assists(), ADR: adr(pl, played),
			Gun: gun, Nades: nades, HasBomb: bomb,
		})
	}

	for _, v := range smokes {
		s.Utility = append(s.Utility, Utility{USmoke, v})
	}
	for _, inf := range gs.Infernos() {
		fires := inf.Fires().Active().List()
		if len(fires) == 0 {
			continue
		}
		var c Vec
		for _, f := range fires {
			c.X += f.X / float64(len(fires))
			c.Y += f.Y / float64(len(fires))
			c.Z += f.Z / float64(len(fires))
		}
		s.Utility = append(s.Utility, Utility{UFire, c})
	}
	for id, g := range gs.GrenadeProjectiles() {
		if g.WeaponInstance == nil {
			continue
		}
		if _, popped := smokes[id]; popped {
			continue
		}
		if k, ok := projectileKind[g.WeaponInstance.Type]; ok {
			s.Utility = append(s.Utility, Utility{k, *vec(g.Position())})
		}
	}

	if rules := gs.Rules(); rules != nil {
		e := rules.Entity()
		if v, ok := prop(e, "m_pGameRules.m_bGamePaused"); ok {
			s.Paused = v.BoolVal()
		}
		start, ok1 := prop(e, "m_pGameRules.m_fRoundStartTime")
		length, ok2 := prop(e, "m_pGameRules.m_iRoundTime")
		if ok1 && ok2 {
			left := float64(start.Float()) - now
			if !s.Freeze {
				left += float64(length.Int())
			}
			s.SecondsLeft = max(0, left)
		}
	}

	s.Bomb = bombState(gs, now)
	return s
}

func bombState(gs *demoinfocs.GameState, now float64) *Bomb {
	for _, e := range gs.Entities() {
		if e == nil || e.ServerClass().Name() != "CPlantedC4" {
			continue
		}
		if v, ok := prop(e, "m_bBombDefused"); ok && v.BoolVal() {
			return nil
		}
		b := &Bomb{Pos: *vec(e.Position()), State: BombPlanted, SecondsLeft: -1, DefuseLeft: -1}
		if v, ok := prop(e, "m_bBeingDefused"); ok && v.BoolVal() {
			b.State = BombDefusing
			if d, ok := prop(e, "m_flDefuseCountDown"); ok {
				b.DefuseLeft = max(0, float64(d.Float())-now)
			}
		}
		if v, ok := prop(e, "m_flC4Blow"); ok {
			b.SecondsLeft = max(0, float64(v.Float())-now)
		}
		return b
	}
	b := gs.Bomb()
	if b == nil || b.Carrier != nil {
		return nil
	}
	pos := b.Position()
	if pos.X == 0 && pos.Y == 0 && pos.Z == 0 {
		return nil
	}
	return &Bomb{Pos: *vec(pos), State: BombDropped, SecondsLeft: -1, DefuseLeft: -1}
}
