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

// roundCodes are m_iMatchStats_RoundResults values, verified against the parsed
// round ends of four CS2 matches (0 = not played).
var roundCodes = map[int]Outcome{
	1: {CT, "Enemy eliminated"}, 3: {CT, "Bomb defused"}, 4: {CT, "Time ran out"},
	5: {T, "Enemy eliminated"}, 6: {T, "Bomb exploded"},
}

// history reads the finished rounds' outcomes; an unknown code ends it (shown as
// unknown rounds by the UI rather than guessed).
func history(rules *st.Entity, played int) []Outcome {
	var out []Outcome
	for i := 0; i < min(played, 30); i++ {
		v, ok := prop(rules, fmt.Sprintf("m_pGameRules.m_iMatchStats_RoundResults.%04d", i))
		if !ok {
			break
		}
		o, known := roundCodes[v.Int()]
		if !known {
			o = Outcome{}
		}
		out = append(out, o)
	}
	return out
}

// roundStats sums the per-round stats of the rounds played (some servers leave the
// m_matchStats totals at 0; the per-round ones are always filled) and returns ADR.
func roundStats(pl *common.Player, rounds int) (float64, Stats) {
	get := func(i int, field string) int {
		if v, ok := prop(pl.Entity, fmt.Sprintf("m_pActionTrackingServices.m_perRoundStats.%04d.%s", i, field)); ok {
			return v.Int()
		}
		return 0
	}
	s := Stats{Rounds: rounds, MVPs: pl.MVPs()}
	dmg := 0
	for i := 0; i < rounds; i++ {
		k := get(i, "m_iKills")
		dmg += get(i, "m_iDamage")
		s.HSKills += get(i, "m_iHeadShotKills")
		s.UtilDamage += get(i, "m_iUtilityDamage")
		s.Flashed += get(i, "m_iEnemiesFlashed")
		if k > 0 || get(i, "m_iAssists") > 0 || get(i, "m_iDeaths") == 0 {
			s.KASRounds++
		}
		if k >= 3 {
			s.Multi[min(k, 5)-3]++
		}
	}
	s.UtilDamage = max(s.UtilDamage, pl.UtilityDamage())
	return float64(max(dmg, pl.TotalDamage())) / float64(max(1, rounds)), s
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
		adrOf, stats := roundStats(pl, played)
		s.Players = append(s.Players, Player{
			Name: clean(pl.Name), Side: side, Pos: pos, Yaw: float64(pl.ViewDirectionX()),
			Alive: pl.IsAlive(), HP: pl.Health(), Armor: pl.Armor(), Helmet: pl.HasHelmet(), Defuser: pl.HasDefuseKit(),
			Money: pl.Money(), Equip: pl.EquipmentValueCurrent(), Kills: pl.Kills(), Deaths: pl.Deaths(), Assists: pl.Assists(), ADR: adrOf, Stats: stats,
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
		s.History = history(rules.Entity(), played)
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
