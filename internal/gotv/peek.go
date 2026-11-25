package gotv

import (
	"context"
	"errors"
	"time"

	demoinfocs "github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs"
	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/common"
	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/cstv"
	"github.com/markus-wa/demoinfocs-golang/v6/pkg/demoinfocs/events"
)

// Summary is who is playing on a relay, for the source list.
type Summary struct {
	CT, T Team
	Round int
	Over  bool
}

const peekTimeout = 15 * time.Second

// Peek reads a relay just far enough (its first full snapshot) to tell the teams,
// score and round, then stops.
func Peek(ctx context.Context, url string) (*Summary, error) {
	ctx, cancel := context.WithTimeout(ctx, peekTimeout)
	defer cancel()
	r, err := cstv.NewReader(ctx, url, cstv.WithHTTPClient(relayClient))
	if err != nil {
		return nil, err
	}
	p := demoinfocs.NewParser(r)
	defer p.Close()
	var out *Summary
	p.RegisterEventHandler(func(events.FrameDone) {
		gs := p.GameState()
		if out != nil || len(gs.Participants().Playing()) == 0 {
			return // wait until the players (and with them the teams) are in
		}
		name := func(n, fallback string) string {
			if n == "" {
				return fallback
			}
			return n
		}
		phase := gs.GamePhase()
		s := &Summary{Round: gs.TotalRoundsPlayed() + 1, Over: phase == common.GamePhaseGameEnded || phase == common.GamePhaseGameOver}
		if ct := gs.TeamCounterTerrorists(); ct != nil {
			s.CT = Team{Name: name(clean(ct.ClanName()), "CT"), Score: ct.Score()}
		}
		if t := gs.TeamTerrorists(); t != nil {
			s.T = Team{Name: name(clean(t.ClanName()), "T"), Score: t.Score()}
		}
		out = s
		cancel()
	})
	err = p.ParseToEnd(ctx)
	if out != nil {
		return out, nil
	}
	if err == nil {
		err = errors.New("no players in the broadcast")
	}
	return nil, err
}
