package gotv

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/golang/geo/r3"
	demoinfocs "github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/cstv"
	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/events"

	"github.com/dormonbear/gotv-tui/internal/feeds"
	"github.com/dormonbear/gotv-tui/internal/playback"
)

const (
	sampleTicks = 4 // 16 samples/s at 64 tick
	playEvery   = time.Second / 16
)

// Handlers receive everything on the playback goroutine.
type Handlers struct {
	State  func(State)
	Event  func(Event)
	Status func(string)
}

func sideOf(t common.Team) Side {
	switch t {
	case common.TeamCounterTerrorists:
		return CT
	case common.TeamTerrorists:
		return T
	}
	return ""
}

func who(p *common.Player) *Who {
	if p == nil || sideOf(p.Team) == "" {
		return nil
	}
	return &Who{Name: p.Name, Side: sideOf(p.Team)}
}

func vec(v r3.Vector) *Vec { return &Vec{v.X, v.Y, v.Z} }

var reasons = map[events.RoundEndReason]string{
	events.RoundEndReasonTargetBombed:        "Bomb exploded",
	events.RoundEndReasonBombDefused:         "Bomb defused",
	events.RoundEndReasonCTWin:               "Enemy eliminated",
	events.RoundEndReasonTerroristsWin:       "Enemy eliminated",
	events.RoundEndReasonTargetSaved:         "Time ran out",
	events.RoundEndReasonTerroristsSurrender: "Surrender",
	events.RoundEndReasonCTSurrender:         "Surrender",
}

// Connect streams a CSTV relay until ctx ends, reconnecting when the relay ends or errors.
func Connect(ctx context.Context, url string, h Handlers) {
	client := &http.Client{Timeout: 10 * time.Second}
	for ctx.Err() == nil {
		s, err := feeds.FetchSync(client, url)
		switch {
		case errors.Is(err, feeds.ErrWaiting):
			h.Status("waiting for the broadcast to start...")
		case err != nil:
			h.Status("relay offline, retrying...")
		default:
			h.Status("connected · " + s.Map)
			err = stream(ctx, url, s, h)
			if ctx.Err() != nil {
				return
			}
			h.Status(fmt.Sprintf("relay ended (%v), reconnecting...", err))
		}
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
	}
}

func stream(ctx context.Context, url string, s *feeds.Sync, h Handlers) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	r, err := cstv.NewReader(ctx, url)
	if err != nil {
		return err
	}
	p := demoinfocs.NewParser(r)
	defer p.Close()
	tps := s.TPS
	if tps <= 0 {
		tps = 64
	}
	pb := playback.New[State, Event](tps, 1.5, 5)
	gs := p.GameState()
	tick := func() int { return gs.IngameTick() }
	emit := func(e Event) { pb.PushEvent(tick(), e) }
	smokes := map[int]Vec{} // live smoke clouds by projectile entity id

	last := -sampleTicks
	p.RegisterEventHandler(func(events.FrameDone) {
		if t := tick(); t >= last+sampleTicks {
			last = t
			pb.PushFrame(t, snapshot(p, s.Map, smokes))
		}
	})
	p.RegisterEventHandler(func(e events.Kill) {
		v := who(e.Victim)
		if v == nil {
			return
		}
		ev := Event{Kind: EvKill, Victim: v, Killer: who(e.Killer), Assister: who(e.Assister), AssistFlash: e.AssistedFlash,
			Headshot: e.IsHeadshot, Wallbang: e.IsWallBang(), ThroughSmoke: e.ThroughSmoke, NoScope: e.NoScope, Blind: e.AttackerBlind,
			To: vec(e.Victim.Position())}
		if e.Weapon != nil {
			ev.Weapon = e.Weapon.String()
		}
		if e.Killer != nil {
			ev.From = vec(e.Killer.Position())
		}
		emit(ev)
	})
	p.RegisterEventHandler(func(events.RoundStart) {
		clear(smokes)
		emit(Event{Kind: EvRoundStart, Round: gs.TotalRoundsPlayed() + 1})
	})
	p.RegisterEventHandler(func(e events.RoundEnd) {
		reason, ok := reasons[e.Reason]
		if !ok {
			reason = e.Message
		}
		emit(Event{Kind: EvRoundEnd, Winner: sideOf(e.Winner), Reason: reason})
	})
	p.RegisterEventHandler(func(e events.BombPlanted) { emit(Event{Kind: EvPlant, Player: name(e.Player)}) })
	p.RegisterEventHandler(func(e events.BombDefused) { emit(Event{Kind: EvDefuse, Player: name(e.Player)}) })
	p.RegisterEventHandler(func(events.BombExplode) { emit(Event{Kind: EvExplode}) })
	p.RegisterEventHandler(func(e events.SmokeStart) { smokes[e.GrenadeEntityID] = *vec(e.Position) })
	p.RegisterEventHandler(func(e events.SmokeExpired) { delete(smokes, e.GrenadeEntityID) })

	go func() {
		t := time.NewTicker(playEvery)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-t.C:
				frame, evs := pb.Step(float64(now.UnixMilli()))
				for _, e := range evs {
					h.Event(e)
				}
				if frame != nil {
					h.State(*frame)
				}
			}
		}
	}()
	return p.ParseToEnd(ctx)
}

func name(p *common.Player) string {
	if p == nil {
		return ""
	}
	return p.Name
}
