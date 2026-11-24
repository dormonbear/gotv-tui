// Package gotv turns a live CSTV (GOTV+) relay into match snapshots and events.
package gotv

type Side string

const (
	CT Side = "CT"
	T  Side = "T"
)

type Vec struct{ X, Y, Z float64 }

// Nade is a carried grenade.
type Nade string

const (
	HE    Nade = "he"
	Flash Nade = "flash"
	Smoke Nade = "smoke"
	Fire  Nade = "fire"
	Decoy Nade = "decoy"
)

type Player struct {
	Name    string
	Side    Side
	Pos     *Vec
	Yaw     float64 // degrees, 0 = +x (east), 90 = +y (north)
	Alive   bool
	HP      int
	Armor   int
	Helmet  bool
	Defuser bool
	Money   int
	Kills   int
	Deaths  int
	Assists int
	ADR     float64
	Gun     string // primary if carried, else pistol
	Nades   []Nade // one entry per grenade
	HasBomb bool
}

type UtilityKind string

const (
	USmoke     UtilityKind = "smoke"     // active smoke cloud
	UFire      UtilityKind = "fire"      // burning inferno point
	UHE        UtilityKind = "he"        // grenades in flight
	UFlash     UtilityKind = "flash"
	UMolotov   UtilityKind = "molotov"
	UDecoy     UtilityKind = "decoy"
	USmokeNade UtilityKind = "smokeNade"
)

type Utility struct {
	Kind UtilityKind
	Pos  Vec
}

type BombState string

const (
	BombDropped  BombState = "dropped"
	BombPlanted  BombState = "planted"
	BombDefusing BombState = "defusing"
)

type Bomb struct {
	Pos         Vec
	State       BombState
	SecondsLeft float64 // < 0 when unknown
}

type Team struct {
	Name  string
	Score int
}

type State struct {
	Map         string
	CT, T       Team
	Round       int
	SecondsLeft float64 // < 0 when unknown
	Freeze      bool
	Warmup      bool
	Paused      bool
	Bomb        *Bomb
	Players     []Player
	Utility     []Utility
}

type Who struct {
	Name string
	Side Side
}

type EventKind string

const (
	EvKill       EventKind = "kill"
	EvRoundStart EventKind = "roundStart"
	EvRoundEnd   EventKind = "roundEnd"
	EvPlant      EventKind = "plant"
	EvDefuse     EventKind = "defuse"
	EvExplode    EventKind = "explode"
)

// Event is one game-log line; only the fields of its Kind are set.
type Event struct {
	Kind EventKind
	// kill
	Killer, Victim, Assister *Who
	AssistFlash              bool
	Weapon                   string
	Headshot, Wallbang       bool
	ThroughSmoke, NoScope    bool
	Blind                    bool
	From, To                 *Vec
	// roundStart
	Round int
	// roundEnd
	Winner Side // "" for a draw
	Reason string
	// plant / defuse
	Player string
}
