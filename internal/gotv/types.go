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
	Equip   int // value of what they carry
	Stats   Stats
	Kills   int
	Deaths  int
	Assists int
	ADR     float64
	Gun     string // primary if carried, else pistol
	Nades   []Nade // one entry per grenade
	HasBomb bool
	Blind   float64 // seconds of flash blindness left
}

// Stats are a player's match totals, summed from the per-round stats.
type Stats struct {
	Rounds     int    // rounds played
	HSKills    int    // headshot kills
	KASRounds  int    // rounds with a kill, an assist or survived (KAST without trades)
	UtilDamage int    // damage done with grenades
	Flashed    int    // enemies flashed
	Multi      [3]int // rounds with 3, 4 and 5 kills
	MVPs       int
}

type UtilityKind string

const (
	USmoke     UtilityKind = "smoke" // active smoke cloud
	UFire      UtilityKind = "fire"  // burning inferno point
	UHE        UtilityKind = "he"    // grenades in flight
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
	DefuseLeft  float64 // < 0 unless being defused
}

type Team struct {
	Name  string
	Score int
}

// Outcome is how a finished round ended.
type Outcome struct {
	Winner Side
	Reason string // as in Event.Reason
}

type State struct {
	Map         string
	CT, T       Team
	Round       int
	SecondsLeft float64 // < 0 when unknown
	Freeze      bool
	Warmup      bool
	Paused      bool
	Over        bool // match finished
	Bomb        *Bomb
	Players     []Player
	Utility     []Utility
	History     []Outcome // finished rounds in order, from the game rules (first 30)
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
	EvBlast      EventKind = "blast" // an HE or flash going off at To; not a log line
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
	Nade                     Nade // blast
	From, To                 *Vec
	// roundStart: the round starting; roundEnd: the round that ended
	Round int
	// roundEnd
	Winner Side // "" for a draw
	Reason string
	// plant / defuse
	Player string
}
