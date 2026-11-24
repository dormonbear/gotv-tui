import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { dirname, join } from 'node:path'

export type Feed = { name: string; url: string }

// Relays organizers have published for in-game viewing (playcast "<url>").
// Sources: PGL Singapore 2026 community guidelines (pglesports.com), BLAST.tv Major
// community guidelines (blast.tv/major/community-guidelines), HLTV stream list for
// PGL Copenhagen 2024. Organizers reuse/replace these per event; edit as needed.
export const PRESETS: Feed[] = [
  { name: 'PGL Major · Stream A', url: 'https://cstv.fragnet.net/a' },
  { name: 'PGL Major · Stream B', url: 'https://cstv.fragnet.net/b' },
  { name: 'BLAST Major · Stream A', url: 'https://gotv.blast.tv/major-a' },
  { name: 'BLAST Major · Stream B', url: 'https://gotv.blast.tv/major-b' },
  { name: 'Challengermode Major · Primary', url: 'https://cstv.challengermode.com/major/primary' },
]

export const FEEDS_FILE = join(homedir(), '.config', 'gotv-tui', 'feeds.json')

export function loadFeeds(file = FEEDS_FILE): Feed[] {
  try {
    const list = JSON.parse(readFileSync(file, 'utf8'))
    if (Array.isArray(list)) return list.filter((f) => typeof f?.name === 'string' && typeof f?.url === 'string')
  } catch {}
  return PRESETS
}

export function saveFeeds(feeds: Feed[], file = FEEDS_FILE) {
  mkdirSync(dirname(file), { recursive: true })
  writeFileSync(file, JSON.stringify(feeds, null, 2) + '\n')
}

export const normalizeUrl = (u: string) => u.trim().replace(/^playcast\s+/, '').replace(/^"|"$/g, '').replace(/\/+$/, '')
export const isRelayUrl = (u: string) => /^https?:\/\/[^\s/]+/.test(normalizeUrl(u))

// live: relay is receiving data; stale: answers but nothing new for a while (match over);
// waiting: relay reset for the next match and has no fragment yet (HTTP 405 "check back soon").
export type FeedState = 'live' | 'stale' | 'waiting' | 'offline'
export type FeedStatus = Feed & { state: FeedState; map: string | null }
const STALE_S = 60

export function feedState(sync: any): FeedState {
  if (!sync || typeof sync.tick !== 'number') return 'offline'
  return typeof sync.rcvage === 'number' && sync.rcvage > STALE_S ? 'stale' : 'live'
}

export async function probeFeed(f: Feed): Promise<FeedStatus> {
  try {
    const r = await fetch(`${f.url}/sync`, { signal: AbortSignal.timeout(8000) })
    if (r.status === 405) return { ...f, state: 'waiting', map: null }
    const j = r.ok ? await r.json() : null
    return { ...f, state: feedState(j), map: j?.map ?? null }
  } catch {
    return { ...f, state: 'offline', map: null }
  }
}
