import { Box, Text } from 'ink'
import { useEffect, useMemo, useState } from 'react'
import { CT, fg, formatEvent, RESET, T } from '../format.ts'
import type { MatchState } from '../gotv.ts'
import { opaqueBox, rasterize, sitesOn, type RadarAssets } from '../maps.ts'
import { buildBase, drawMinimap, type KillMark, type Level } from '../minimap.ts'
import { playerNumbers, TeamTable } from './Scoreboard.tsx'
import { useMatch } from './useMatch.ts'

const PANEL_ROWS = 1 + 6 + 6 + 1 // header, CT table, T table, spacer
const LEFT_W = 76

function useNow(ms: number) {
  const [now, setNow] = useState(Date.now())
  useEffect(() => {
    const t = setInterval(() => setNow(Date.now()), ms)
    return () => clearInterval(t)
  }, [ms])
  return now
}

const mmss = (s: number) => `${Math.floor(s / 60)}:${String(Math.floor(s % 60)).padStart(2, '0')}`

function header(st: MatchState) {
  const phase = st.warmup ? ' warmup' : st.paused ? ' paused' : st.freeze ? ' freeze' : ''
  const bomb = st.bomb && st.bomb.state !== 'dropped'
    ? `  \x1b[41;1m ${st.bomb.state === 'defusing' ? 'DEFUSING' : 'BOMB'}${st.bomb.secondsLeft != null ? ' ' + Math.ceil(st.bomb.secondsLeft) + 's' : ''} \x1b[0m`
    : ''
  return (
    `${fg(CT)}\x1b[1m ${st.ct.name} ${st.ct.score}${RESET} : ${fg(T)}\x1b[1m${st.t.score} ${st.t.name}${RESET}` +
    `  \x1b[1m${st.secondsLeft != null ? mmss(st.secondsLeft) : '--:--'}${RESET}  \x1b[2mR${st.round} ${st.map}${phase}${RESET}${bomb}`
  )
}

type LevelProps = {
  assets: RadarAssets
  level: Level
  st: MatchState | null
  kills: KillMark[]
  nums: Map<string, number>
  now: number
  width: number
  height: number // rows including border and title
}

// One framed radar for one level; only entities on that level are drawn.
function LevelMap({ assets, level, st, kills, nums, now, width, height }: LevelProps) {
  const png = level === 'lower' && assets.lower ? assets.lower : assets.upper
  // both levels use the upper radar's crop so they share scale and line up vertically
  const box = useMemo(() => opaqueBox([assets.upper]), [assets.upper])
  const titled = !!assets.lower // single-level maps skip the title row (map name is in the header)
  const radar = useMemo(() => rasterize(png, assets.overview, box, (width - 2) * 2, (height - (titled ? 3 : 2)) * 4), [png, box, width, height, titled])
  const ov = assets.overview
  const base = useMemo(() => buildBase(radar), [radar])
  const lines = drawMinimap(radar, base, st, { kills, nums, now, level, lowerBelow: ov.lowerBelow, sites: sitesOn(assets.map, level, ov.sites) })
  return (
    <Box flexDirection="column" borderStyle="round" borderColor="gray">
      {titled && <Text dimColor wrap="truncate">{` ${level}`}</Text>}
      {lines.map((l, i) => (
        <Text key={i}>{l}</Text>
      ))}
    </Box>
  )
}

type Props = { url: string; cols: number; rows: number }

// Layout: left = header + scoreboard on top, game log below (fixed width); right = minimap
// (two-level maps show both levels side by side).
export function MatchView({ url, cols, rows }: Props) {
  const d = useMatch(url)
  const now = useNow(250)
  const leftW = Math.min(LEFT_W, Math.floor(cols / 2)) // scoreboard needs ~76 cols; the map gets the rest
  const mapW = cols - leftW - 1
  const bodyRows = rows - 1 // footer
  const levels: Level[] = d.radar?.lower ? ['upper', 'lower'] : ['upper']
  const levelW = Math.floor(mapW / levels.length) // two-level maps sit side by side
  const players = d.st?.players ?? []
  const nums = useMemo(() => playerNumbers(players), [players.map((p) => p.name).join()])
  const log = d.log.slice(0, Math.max(0, bodyRows - PANEL_ROWS - 1)).map(formatEvent)
  const st = d.st

  return (
    <Box flexDirection="column" width={cols} height={rows}>
      <Box height={bodyRows}>
        <Box flexDirection="column" width={leftW}>
          {st ? <Text wrap="truncate">{header(st)}</Text> : <Text dimColor> {d.status} · waiting for game state...</Text>}
          {st && (
            <>
              <TeamTable side="CT" team={st.ct} players={players} nums={nums} width={leftW} />
              <TeamTable side="T" team={st.t} players={players} nums={nums} width={leftW} />
            </>
          )}
          <Text> </Text>
          <Text bold>Game log</Text>
          {log.length === 0 && <Text dimColor>events appear from the moment you connect</Text>}
          {log.map((l, i) => (
            <Text key={i} wrap="truncate">
              {l}
            </Text>
          ))}
        </Box>
        <Box width={mapW} marginLeft={1} justifyContent="center" alignItems="center">
          {d.radar ? (
            levels.map((lv) => <LevelMap key={lv} assets={d.radar!} level={lv} st={st} kills={d.kills} nums={nums} now={now} width={levelW} height={bodyRows} />)
          ) : (
            <Text dimColor> loading map...</Text>
          )}
        </Box>
      </Box>
      <Text dimColor wrap="truncate">
        {` q quit · b sources · ${d.status} · CSTV ~3min delay`}
      </Text>
    </Box>
  )
}
