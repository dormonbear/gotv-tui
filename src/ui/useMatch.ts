import { useEffect, useState } from 'react'
import { connectGotv, type LogEvent, type MatchState } from '../gotv.ts'
import { loadRadar, type RadarAssets } from '../maps.ts'
import type { KillMark } from '../minimap.ts'

export type MatchData = {
  st: MatchState | null
  log: LogEvent[] // newest first
  kills: KillMark[] // current round
  status: string
  radar: RadarAssets | null
}

const LOG_LIMIT = 200

export function applyEvent(d: MatchData, e: LogEvent, now: number): MatchData {
  const log = [e, ...d.log].slice(0, LOG_LIMIT)
  if (e.type === 'roundStart') return { ...d, log, kills: [] }
  if (e.type === 'kill' && e.to) return { ...d, log, kills: [...d.kills, { from: e.from, to: e.to, victim: e.victim.side, at: now }] }
  return { ...d, log }
}

export function useMatch(url: string): MatchData {
  const [d, setD] = useState<MatchData>({ st: null, log: [], kills: [], status: 'connecting...', radar: null })

  useEffect(
    () =>
      connectGotv(url, {
        onState: (st) => setD((p) => ({ ...p, st })),
        onEvent: (e) => setD((p) => applyEvent(p, e, Date.now())),
        onStatus: (status) => setD((p) => ({ ...p, status })),
      }),
    [url],
  )

  const map = d.st?.map
  useEffect(() => {
    if (!map || d.radar?.map === map) return
    loadRadar(map).then(
      (radar) => setD((p) => ({ ...p, radar })),
      (e) => setD((p) => ({ ...p, status: `map ${map}: ${e.message}` })),
    )
  }, [map])

  return d
}
