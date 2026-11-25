package gotv

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/golang/geo/r3"
	demoinfocs "github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/cstv"
	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/events"

	"github.com/dormonbear/gotv-tui/internal/feeds"
	"github.com/dormonbear/gotv-tui/internal/playback"
	"github.com/dormonbear/gotv-tui/internal/term"
)

const (
	sampleTicks = 4 // 16 samples/s at 64 tick
	playEvery   = time.Second / 16
	// Lookahead is how far ahead of the screen the parser runs. The director uses it
	// to see a fight coming (like Valve's HLTV director, which reads 8 s ahead).
	Lookahead = 4.0
)

// Handlers receive everything on the playback goroutine.
type Handlers struct {
	State  func(State)
	Event  func(Event)
	Status func(string)
	// Ahead gets the events due in the next Lookahead seconds and the latest
	// queued state in that window, after every playback step.
	Ahead func(soon []playback.Soon[Event], future *State)
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
	return &Who{Name: clean(p.Name), Side: sideOf(p.Team)}
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
		s, err := feeds.FetchSync(ctx, client, url)
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

// relayClient caps every relay response: a hostile or broken relay can't make us
// buffer an endless fragment.
var relayClient = &http.Client{Timeout: 30 * time.Second, Transport: capped{http.DefaultTransport}}

const maxFragment = 64 << 20

type capped struct{ rt http.RoundTripper }

func (c capped) RoundTrip(req *http.Request) (*http.Response, error) {
	r, err := c.rt.RoundTrip(req)
	if err == nil {
		r.Body = &limitBody{r.Body, maxFragment}
	}
	return r, err
}

type limitBody struct {
	io.ReadCloser
	left int64
}

func (l *limitBody) Read(p []byte) (int, error) {
	if l.left <= 0 {
		return 0, errors.New("relay response too large")
	}
	if int64(len(p)) > l.left {
		p = p[:l.left]
	}
	n, err := l.ReadCloser.Read(p)
	l.left -= int64(n)
	return n, err
}

func stream(parent context.Context, url string, s *feeds.Sync, h Handlers) (err error) {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	defer func() { // a parser panic must not take the terminal down with it
		if r := recover(); r != nil {
			err = fmt.Errorf("relay parser crashed: %v", r)
		}
	}()
	r, err := cstv.NewReader(ctx, url, cstv.WithHTTPClient(relayClient))
	if err != nil {
		return err
	}
	p := demoinfocs.NewParser(r)
	defer p.Close()
	tps := s.TPS
	if tps <= 0 {
		tps = 64
	}
	pb := playback.New[State, Event](tps, Lookahead, Lookahead+4)
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
			ev.Weapon = clean(e.Weapon.String())
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
			reason = clean(e.Message)
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
				if h.Ahead != nil {
					h.Ahead(pb.Ahead(Lookahead))
				}
			}
		}
	}()
	err = p.ParseToEnd(ctx)
	if parent.Err() == nil { // the relay ended, the viewer didn't leave: play out the buffer
		deadline := time.After(time.Duration((Lookahead + 4) * float64(time.Second)))
		for pb.Pending() {
			select {
			case <-parent.Done():
				return err
			case <-deadline:
				return err
			case <-time.After(100 * time.Millisecond):
			}
		}
	}
	return err
}

// clean bounds a name from the relay to printable text (see term.Clean).
func clean(s string) string { return term.Clean(s, 24) }

func name(p *common.Player) string {
	if p == nil {
		return ""
	}
	return clean(p.Name)
}
