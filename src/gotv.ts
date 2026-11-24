import { DemoReader, EntityMode, WinRoundReason } from 'cs2parser'
import { Playback } from './playback.ts'

export type Side = 'T' | 'CT'
export type Vec = { x: number; y: number; z: number }
export type PlayerState = {
  name: string
  side: Side
  pos: Vec | null
  yaw: number // degrees, 0 = +x (east), 90 = +y (north)
  alive: boolean
  hp: number
  armor: number
  helmet: boolean
  defuser: boolean
  money: number
  kills: number
  deaths: number
  assists: number
  adr: number
  weapon: string | null
  hasBomb: boolean
}
export type UtilityKind = 'smoke' | 'fire' | 'he' | 'flash' | 'molotov' | 'decoy' | 'smokeNade'
export type Utility = { kind: UtilityKind; pos: Vec }
export type Bomb = { pos: Vec; state: 'dropped' | 'planted' | 'defusing'; secondsLeft: number | null }
export type MatchState = {
  map: string
  ct: { name: string; score: number }
  t: { name: string; score: number }
  round: number
  secondsLeft: number | null
  freeze: boolean
  warmup: boolean
  paused: boolean
  bomb: Bomb | null
  players: PlayerState[]
  utility: Utility[]
}
type Who = { name: string; side: Side }
export type LogEvent =
  | {
      type: 'kill'
      killer: Who | null
      victim: Who
      assister: (Who & { flash: boolean }) | null
      weapon: string
      headshot: boolean
      wallbang: boolean
      smoke: boolean
      noscope: boolean
      blind: boolean
      from: Vec | null
      to: Vec | null
    }
  | { type: 'roundStart'; round: number }
  | { type: 'roundEnd'; winner: Side | null; reason: string }
  | { type: 'plant'; player: string | null }
  | { type: 'defuse'; player: string | null }
  | { type: 'explode' }

const side = (team: number): Side | null => (team === 3 ? 'CT' : team === 2 ? 'T' : null)
const copy = (v: { x: number; y: number; z: number } | null | undefined): Vec | null => (v ? { x: v.x, y: v.y, z: v.z } : null)
const who = (p: any): Who | null => {
  const s = p && side(p.teamNumber)
  return s ? { name: p.name, side: s } : null
}

const REASON: Record<number, string> = {
  [WinRoundReason.TARGET_BOMBED]: 'Bomb exploded',
  [WinRoundReason.BOMB_DEFUSED]: 'Bomb defused',
  [WinRoundReason.T_ELIMINATED]: 'Enemy eliminated',
  [WinRoundReason.CT_ELIMINATED]: 'Enemy eliminated',
  [WinRoundReason.TARGET_SAVED]: 'Time ran out',
  [WinRoundReason.T_SURRENDER]: 'Surrender',
  [WinRoundReason.CT_SURRENDER]: 'Surrender',
}

const NADES: [string, UtilityKind][] = [
  ['CHEGrenadeProjectile', 'he'],
  ['CFlashbangProjectile', 'flash'],
  ['CMolotovProjectile', 'molotov'],
  ['CDecoyProjectile', 'decoy'],
  ['CSmokeGrenadeProjectile', 'smokeNade'],
]

// Source 2 networks origins as cell + offset: world = cell * 512 - 16384 + vec.
const prefixes = new Map<string, string>()
export function entityOrigin(props: Record<string, any>): Vec | null {
  const keys = Object.keys(props)
  const cls = keys[0]?.split('.')[0] ?? ''
  let pre = prefixes.get(cls)
  if (!pre) {
    const k = keys.find((k) => k.endsWith('m_vecOrigin.m_cellX'))
    if (!k) return null
    pre = k.slice(0, -'m_cellX'.length)
    prefixes.set(cls, pre)
  }
  const axis = (a: string) => props[`${pre}m_cell${a}`] * 512 - 16384 + props[`${pre}m_vec${a}`]
  const v = { x: axis('X'), y: axis('Y'), z: axis('Z') }
  return Number.isFinite(v.x) ? v : null
}

function snapshot(p: DemoReader, map: string, tps: number): MatchState {
  const gr = p.gameRules
  const now = p.currentTick / tps
  const teams = p.teams
  const team = (n: number) => {
    const t = teams.find((t) => t.teamNumber === n)
    return { name: t?.clanName || (n === 3 ? 'CT' : 'T'), score: t?.score ?? 0 }
  }
  const rounds = Math.max(1, gr?.roundsPlayed ?? 1)
  const players: PlayerState[] = []
  for (const c of p.playerControllers) {
    const s = side(c.teamNumber)
    if (!s || c.isHLTV) continue
    const inv = c.inventory
    players.push({
      name: c.name,
      side: s,
      pos: copy(c.position),
      yaw: c.eyeAngles.yaw,
      alive: c.isAlive,
      hp: c.health,
      armor: c.armor,
      helmet: c.hasHelmet,
      defuser: c.hasDefuser,
      money: c.money,
      kills: c.kills,
      deaths: c.deaths,
      assists: c.assists,
      // m_matchStats damage is left at 0 by some servers; per-round stats are always filled
      adr: Math.max(c.damage, c.perRoundStats.reduce((a: number, r: any) => a + (r?.m_iDamage ?? 0), 0)) / rounds,
      weapon: inv?.activeItem?.name ?? null,
      hasBomb: inv?.items.some((i) => i.className === 'CC4') ?? false,
    })
  }

  const utility: Utility[] = []
  for (const s of p.smokes) {
    const d = copy(s.detonationPos)
    if (d) utility.push({ kind: 'smoke', pos: d })
  }
  const smokeIds = new Set(p.smokes.map((s: any) => s.entityId))
  for (const { properties } of p.findEntities('CInferno')) {
    const o = entityOrigin(properties)
    if (o) utility.push({ kind: 'fire', pos: o })
  }
  for (const [cls, kind] of NADES)
    for (const { entityId, properties } of p.findEntities(cls as any)) {
      if (kind === 'smokeNade' && smokeIds.has(entityId)) continue
      const o = entityOrigin(properties)
      if (o) utility.push({ kind, pos: o })
    }

  let bomb: Bomb | null = null
  const planted = p.findEntities('CPlantedC4')[0]
  if (planted) {
    const pr = planted.properties as Record<string, any>
    const o = entityOrigin(pr)
    if (o && !pr['CPlantedC4.m_bBombDefused'])
      bomb = {
        pos: o,
        state: pr['CPlantedC4.m_bBeingDefused'] ? 'defusing' : 'planted',
        secondsLeft: typeof pr['CPlantedC4.m_flC4Blow'] === 'number' ? Math.max(0, pr['CPlantedC4.m_flC4Blow'] - now) : null,
      }
  } else if (!players.some((pl) => pl.alive && pl.hasBomb)) {
    const c4 = p.findEntities('CC4')[0]
    const o = c4 && entityOrigin(c4.properties)
    if (o && (o.x !== -16384 || o.y !== -16384)) bomb = { pos: o, state: 'dropped', secondsLeft: null }
  }

  const freeze = gr?.isFreezePeriod ?? false
  const left = gr ? (freeze ? gr.roundStartTime - now : gr.roundStartTime + gr.roundTime - now) : null
  return {
    map,
    ct: team(3),
    t: team(2),
    round: (gr?.roundsPlayed ?? 0) + 1,
    secondsLeft: left == null ? null : Math.max(0, left),
    freeze,
    warmup: gr?.isWarmup ?? false,
    paused: gr?.isGamePaused ?? false,
    bomb,
    players,
    utility,
  }
}

type Handlers = {
  onState: (s: MatchState) => void
  onEvent: (e: LogEvent) => void
  onStatus: (s: string) => void
}

const SAMPLE_TICKS = 4 // 16 samples/s at 64 tick
const PLAY_MS = 1000 / 16

// Streams a CSTV/GOTV+ relay until stopped; reconnects when the relay ends or errors.
export function connectGotv(url: string, h: Handlers): () => void {
  const ac = new AbortController()
  const run = async () => {
    while (!ac.signal.aborted) {
      const res = await fetch(`${url}/sync`, { signal: ac.signal }).catch(() => null)
      const sync = res?.ok ? await res.json().catch(() => null) : null
      if (ac.signal.aborted) return
      if (!sync) {
        h.onStatus(res?.status === 405 ? 'waiting for the broadcast to start...' : 'relay offline, retrying...')
        await sleep(5000, ac.signal)
        continue
      }
      const map: string = sync.map
      h.onStatus(`connected · ${map}`)
      const tps: number = sync.tps || 64
      const p = new DemoReader()
      const pb = new Playback<MatchState, LogEvent>(tps)
      const emit = (e: LogEvent) => pb.pushEvent(p.currentTick, e)
      p.on('tickend', (tick) => {
        if (tick % SAMPLE_TICKS === 0) pb.pushFrame(tick, snapshot(p, map, tps))
      })
      const timer = setInterval(() => {
        const due = pb.step(Date.now())
        due.events.forEach(h.onEvent)
        if (due.frame) h.onState(due.frame)
      }, PLAY_MS)
      p.gameEvents.on('player_death', (e) => {
        const victim = who(e.player)
        if (!victim) return
        const a = who(e.assisterPlayer)
        emit({
          type: 'kill',
          killer: who(e.attackerPlayer),
          victim,
          assister: a ? { ...a, flash: e.assistedflash } : null,
          weapon: e.weapon,
          headshot: e.headshot,
          wallbang: e.penetrated > 0,
          smoke: e.thrusmoke,
          noscope: e.noscope,
          blind: e.attackerblind,
          from: copy(e.attackerPlayer?.position),
          to: copy(e.player?.position),
        })
      })
      p.gameEvents.on('round_start', () => emit({ type: 'roundStart', round: (p.gameRules?.roundsPlayed ?? 0) + 1 }))
      p.gameEvents.on('round_end', (e) => emit({ type: 'roundEnd', winner: side(e.winner), reason: REASON[e.reason] ?? e.message }))
      p.gameEvents.on('bomb_planted', (e) => emit({ type: 'plant', player: e.player?.name ?? null }))
      p.gameEvents.on('bomb_defused', (e) => emit({ type: 'defuse', player: e.player?.name ?? null }))
      p.gameEvents.on('bomb_exploded', () => emit({ type: 'explode' }))
      try {
        const out = await p.parseHttpBroadcast(url, { entities: EntityMode.ALL, signal: ac.signal, onFragmentError: () => 'continue' })
        if (out.status === 'cancelled') return
        h.onStatus(`relay ${out.status}, reconnecting...`)
      } catch (e) {
        if (ac.signal.aborted) return
        h.onStatus(`error: ${(e as Error).message}, reconnecting...`)
      } finally {
        clearInterval(timer)
      }
      await sleep(3000, ac.signal)
    }
  }
  void run()
  return () => ac.abort()
}

const sleep = (ms: number, signal: AbortSignal) =>
  new Promise<void>((r) => {
    const t = setTimeout(r, ms)
    signal.addEventListener('abort', () => (clearTimeout(t), r()), { once: true })
  })
