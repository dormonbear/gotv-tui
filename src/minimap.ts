import { bg, CT, fg, RESET, T, type RGB } from './format.ts'
import type { MatchState, Side, UtilityKind, Vec } from './gotv.ts'
import { isFloor, type Radar } from './maps.ts'

export type KillMark = { from: Vec | null; to: Vec; victim: Side; at: number }
export type Level = 'upper' | 'lower'
type Glyph = { ch: string; fg: RGB; bg?: RGB; bold?: boolean }

const KILL_MS = 4000
const SMOKE_R = 144 // world units
const FIRE_R = 120
const RED: RGB = [235, 60, 60]
const SMOKE: RGB = [175, 178, 186]
const FIRE: RGB = [235, 115, 35]
const BLACK: RGB = [10, 10, 10]
const WHITE: RGB = [255, 255, 255]
const SITE: RGB = [255, 150, 60]
const NADE: Partial<Record<UtilityKind, RGB>> = { he: RED, flash: WHITE, molotov: FIRE, decoy: [120, 200, 120], smokeNade: SMOKE }
const DOT = [[0x1, 0x8], [0x2, 0x10], [0x4, 0x20], [0x40, 0x80]] // braille bit per (dy, dx)
const ARROWS = ['→', '↗', '↑', '↖', '←', '↙', '↓', '↘']

const teamColor = (s: Side) => (s === 'CT' ? CT : T)
const mix = (a: RGB, b: RGB, t: number): RGB => [0, 1, 2].map((i) => Math.round(a[i] * (1 - t) + b[i] * t)) as RGB
const scale = (c: RGB, k: number): RGB => c.map((v) => Math.round(v * k)) as RGB

export const levelOf = (z: number, lowerBelow: number | null): Level => (lowerBelow != null && z < lowerBelow ? 'lower' : 'upper')

// Arrow glyph and neighbouring cell offset for a view direction (yaw in degrees, 0 = east, 90 = north).
export function facing(yaw: number): { ch: string; dx: number; dy: number } {
  const i = ((Math.round(yaw / 45) % 8) + 8) % 8
  const a = (i * Math.PI) / 4
  return { ch: ARROWS[i], dx: Math.round(Math.cos(a)), dy: -Math.round(Math.sin(a)) }
}

type Opts = { kills: KillMark[]; nums: Map<string, number>; now: number; level: Level; lowerBelow: number | null; sites: { name: string; x: number; y: number }[] }

export type Base = { CW: number; CH: number; bg: (RGB | null)[]; dots: Uint8Array; ink: RGB[] }

const sat = (p: Uint8Array, i: number) => Math.max(p[i], p[i + 1], p[i + 2]) - Math.min(p[i], p[i + 1], p[i + 2])

// Static map layer, computed once per radar, on the supersampled image:
// - roofs (low-saturation grey) covering most of a cell paint it as a light block, so
//   buildings read as solid shapes and small crates (under half a cell) drop out;
// - other cells take the average floor colour, darkened so markers pop;
// - braille dots trace only the map outline / gaps between rooms (thin walls survive
//   supersampling) and saturated painted lines (bombsite / spawn boxes), in their colour.
export function buildBase(r: Radar): Base {
  const { width: W, height: H, ss, pixels: p } = r
  const IW = W * ss, IH = H * ss
  const floor = (x: number, y: number) => x >= 0 && y >= 0 && x < IW && y < IH && isFloor(p, y * IW + x)
  const dist = (i: number, j: number) => Math.abs(p[i] - p[j]) + Math.abs(p[i + 1] - p[j + 1]) + Math.abs(p[i + 2] - p[j + 2])
  const roof = (i: number) => sat(p, i) < 16 && p[i] + p[i + 1] + p[i + 2] > 240 // olive outdoor floor sums ~212, roofs ~280
  const edge = (x: number, y: number) => {
    const i = (y * IW + x) * 4
    for (const [dx, dy] of [[1, 0], [-1, 0], [0, 1], [0, -1]]) {
      if (!floor(x + dx, y + dy)) return true // outline / gap between rooms
      const j = ((y + dy) * IW + x + dx) * 4
      if ((sat(p, i) > 60 || sat(p, j) > 60) && dist(i, j) > 60) return true // painted site/spawn boxes
    }
    return false
  }
  const CW = Math.ceil(W / 2), CH = Math.ceil(H / 4)
  const bg: (RGB | null)[] = new Array(CW * CH).fill(null)
  const dots = new Uint8Array(CW * CH)
  const ink: RGB[] = new Array(CW * CH)
  const half = (ss * ss) / 2
  for (let cy = 0; cy < CH; cy++)
    for (let cx = 0; cx < CW; cx++) {
      let floorDots = 0, code = 0, n = 0, rn = 0, en = 0
      const sum = [0, 0, 0], rsum = [0, 0, 0], esum = [0, 0, 0]
      for (let dy = 0; dy < 4; dy++)
        for (let dx = 0; dx < 2; dx++) {
          let fp = 0, ep = 0
          for (let sy = 0; sy < ss; sy++)
            for (let sx = 0; sx < ss; sx++) {
              const x = (cx * 2 + dx) * ss + sx, y = (cy * 4 + dy) * ss + sy
              if (!floor(x, y)) continue
              const i = (y * IW + x) * 4
              fp++, n++
              sum[0] += p[i], sum[1] += p[i + 1], sum[2] += p[i + 2]
              if (roof(i)) (rn++, (rsum[0] += p[i]), (rsum[1] += p[i + 1]), (rsum[2] += p[i + 2]))
              if (edge(x, y)) (ep++, en++, (esum[0] += p[i]), (esum[1] += p[i + 1]), (esum[2] += p[i + 2]))
            }
          if (fp >= half) floorDots++
          if (ep) code |= DOT[dy][dx]
        }
      const i = cy * CW + cx
      dots[i] = code
      if (rn >= n / 2 && rn > 0) bg[i] = scale(rsum.map((v) => v / rn) as RGB, 0.95) // building
      else if (floorDots >= 4) bg[i] = scale(sum.map((v) => v / n) as RGB, 0.62)
      // ink: the line's own colour, lifted so it reads on the darkened floor
      if (en) ink[i] = mix(esum.map((v) => v / en) as RGB, WHITE, 0.35)
    }
  return { CW, CH, bg, dots, ink }
}

// Per-frame overlay: markers on `level` drawn over a copy of the static base.
export function drawMinimap(r: Radar, base: Base, st: MatchState | null, o: Opts): string[] {
  const { CW, CH, dots: cellDots, ink: cellInk } = base
  const cellBg = base.bg.slice()

  const toCell = (v: Vec): [number, number] => {
    const [px, py] = r.toPx(v.x, v.y)
    return [Math.floor(px / 2), Math.floor(py / 4)]
  }
  const here = (v: Vec) => levelOf(v.z, o.lowerBelow) === o.level
  const unit = Math.abs(r.toPx(1000, 0)[0] - r.toPx(0, 0)[0]) / 1000 / 2 // cells per world unit (x)
  const tint = (v: Vec, rad: number, c: RGB, t: number) => {
    const [cx, cy] = toCell(v)
    const rc = Math.max(1, rad * unit)
    for (let yy = Math.floor(cy - rc / 2); yy <= cy + rc / 2; yy++)
      for (let xx = Math.floor(cx - rc); xx <= cx + rc; xx++) {
        const d = (xx - cx) ** 2 + ((yy - cy) * 2) ** 2
        if (xx < 0 || yy < 0 || xx >= CW || yy >= CH || d > rc * rc) continue
        const i = yy * CW + xx
        cellBg[i] = mix(cellBg[i] ?? BLACK, c, t * (1 - (0.35 * d) / (rc * rc))) // soft edge
      }
  }
  const glyphs = new Map<number, Glyph>()
  const putCell = (cx: number, cy: number, g: Glyph, over: boolean) => {
    const cell = cy * CW + cx
    if (cx < 0 || cx >= CW || cy < 0 || cy >= CH || (!over && glyphs.has(cell))) return
    glyphs.set(cell, g)
  }
  const put = (v: Vec, g: Glyph, over = true) => {
    if (!here(v)) return // other level is drawn on its own map
    const [cx, cy] = toCell(v)
    putCell(cx, cy, g, over)
  }

  for (const s of o.sites) {
    const [px, py] = r.normPx(s.x, s.y)
    putCell(Math.floor(px / 2), Math.floor(py / 4), { ch: s.name, fg: SITE, bold: true }, true)
  }
  if (st) {
    for (const u of st.utility) {
      if (!here(u.pos)) continue
      if (u.kind === 'smoke') tint(u.pos, SMOKE_R, SMOKE, 0.75)
      if (u.kind === 'fire') tint(u.pos, FIRE_R, FIRE, 0.85)
    }
  }
  for (const k of o.kills) {
    if (o.now - k.at > KILL_MS || !k.from) continue
    for (let i = 1; i < 24; i++) {
      const t = i / 24
      put({ x: k.from.x + (k.to.x - k.from.x) * t, y: k.from.y + (k.to.y - k.from.y) * t, z: k.to.z }, { ch: '·', fg: RED, bold: true }, false)
    }
  }
  if (st) {
    for (const u of st.utility) {
      const c = NADE[u.kind]
      if (c) put(u.pos, { ch: '•', fg: c, bold: true })
    }
    if (st.bomb) put(st.bomb.pos, { ch: 'B', fg: BLACK, bg: st.bomb.state === 'dropped' ? [200, 200, 200] : RED, bold: true })
    for (const pl of st.players) if (!pl.alive && pl.pos) put(pl.pos, { ch: '✕', fg: mix(teamColor(pl.side), BLACK, 0.2), bold: true }, false)
    for (const pl of st.players) {
      if (!pl.alive || !pl.pos) continue
      const [cx, cy] = toCell(pl.pos)
      const f = facing(pl.yaw)
      if (here(pl.pos)) putCell(cx + f.dx, cy + f.dy, { ch: f.ch, fg: teamColor(pl.side), bold: true }, false)
    }
    for (const pl of st.players) {
      if (!pl.alive || !pl.pos) continue
      const n = o.nums.get(pl.name.toLowerCase())
      put(pl.pos, { ch: n ? String(n) : '●', fg: pl.hasBomb ? RED : BLACK, bg: teamColor(pl.side), bold: true })
    }
  }

  const rows: string[] = []
  for (let cy = 0; cy < CH; cy++) {
    let line = ''
    for (let cx = 0; cx < CW; cx++) {
      const i = cy * CW + cx
      const g = glyphs.get(i)
      const b = g?.bg ?? cellBg[i]
      if (g) line += (b ? bg(b) : '') + fg(g.fg) + (g.bold ? '\x1b[1m' : '') + g.ch + RESET
      else if (cellDots[i]) line += (b ? bg(b) : '') + fg(cellInk[i]) + String.fromCharCode(0x2800 + cellDots[i]) + RESET
      else line += b ? bg(b) + ' ' + RESET : ' '
    }
    rows.push(line)
  }
  return rows
}
