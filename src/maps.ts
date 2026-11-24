import { Resvg } from '@resvg/resvg-js'
import { mkdirSync, readFileSync, writeFileSync } from 'node:fs'
import { homedir } from 'node:os'
import { join } from 'node:path'

// Valve radar PNGs + overview files (pos_x/pos_y/scale/verticalsections), dumped
// from the game. Fetched once and cached; not bundled (Valve assets).
const SRC = 'https://raw.githubusercontent.com/2mlml/cs2-radar-images/master'
const CACHE = join(homedir(), '.cache', 'gotv-tui', 'radar')

export type Overview = {
  posX: number
  posY: number
  scale: number
  lowerBelow: number | null
  sites: { name: string; x: number; y: number }[] // normalized 0..1 radar coords
}
export type RadarAssets = { map: string; overview: Overview; upper: Buffer; lower: Buffer | null }

export type Radar = {
  width: number // braille dots; one terminal cell = 2 x 4 dots
  height: number
  ss: number // supersampling: each dot is ss x ss image pixels
  pixels: Uint8Array // RGBA, (width * ss) x (height * ss)
  toPx: (x: number, y: number) => [number, number] // world -> dots
  normPx: (nx: number, ny: number) => [number, number] // 0..1 radar coords -> dots
}

// Thin walls in the radars are a few source pixels wide; render at SS x the dot grid
// so they survive and can be traced before downsampling.
const SS = 4

export function parseOverview(txt: string): Overview {
  const num = (k: string) => Number(txt.match(new RegExp(`"${k}"\\s+"(-?[\\d.]+)"`))?.[1])
  const lower = txt.match(/"lower"[^{]*\{[^}]*"AltitudeMax"\s+"(-?[\d.]+)"/)
  const sites = ['A', 'B']
    .map((name) => ({ name, x: num(`bomb${name}_x`), y: num(`bomb${name}_y`) }))
    .filter((s) => Number.isFinite(s.x) && Number.isFinite(s.y))
  return { posX: num('pos_x'), posY: num('pos_y'), scale: num('scale'), lowerBelow: lower ? Number(lower[1]) : null, sites }
}

async function cached(name: string, optional = false): Promise<Buffer | null> {
  const file = join(CACHE, name)
  try {
    return readFileSync(file)
  } catch {}
  const r = await fetch(`${SRC}/${name}`)
  if (r.status === 404 && optional) return null
  if (!r.ok) throw new Error(`radar ${name}: HTTP ${r.status}`)
  const buf = Buffer.from(await r.arrayBuffer())
  mkdirSync(CACHE, { recursive: true })
  writeFileSync(file, buf)
  return buf
}

export async function loadRadar(map: string): Promise<RadarAssets> {
  const [txt, upper, lower] = await Promise.all([cached(`${map}.txt`), cached(`${map}.png`), cached(`${map}_lower.png`, true)])
  return { map, overview: parseOverview(txt!.toString('utf8')), upper: upper!, lower }
}

// Overview files don't say which level a bombsite is on; default is the main (upper) radar.
const LOWER_SITES: Record<string, string[]> = { de_nuke: ['B'] }
export const sitesOn = (map: string, level: 'upper' | 'lower', sites: Overview['sites']) =>
  sites.filter((s) => (LOWER_SITES[map]?.includes(s.name) ? 'lower' : 'upper') === level)

export type Box = { x: number; y: number; w: number; h: number } // in 1024 radar space

// Floor = opaque and not near-black (lower-level radars ship a solid dark backdrop).
export const isFloor = (px: Uint8Array, i: number) =>
  px[i * 4 + 3] > 40 && px[i * 4] * 0.3 + px[i * 4 + 1] * 0.59 + px[i * 4 + 2] * 0.11 >= 40

const svgOf = (png: Buffer, b: Box) =>
  `<svg xmlns="http://www.w3.org/2000/svg" width="${b.w}" height="${b.h}" viewBox="${b.x} ${b.y} ${b.w} ${b.h}">` +
  `<image width="1024" height="1024" href="data:image/png;base64,${png.toString('base64')}"/></svg>`

// Bounding box of the floor area of all images (radars carry wide empty margins).
// Sparse rows/columns (< 8% of the densest one) are ignored so stray specks don't widen it.
export function opaqueBox(pngs: Buffer[]): Box {
  const N = 256
  const EDGE = 8
  const rows = new Array(N).fill(0), cols = new Array(N).fill(0)
  for (const png of pngs) {
    const img = new Resvg(svgOf(png, { x: 0, y: 0, w: 1024, h: 1024 }), { fitTo: { mode: 'width', value: N } }).render()
    // skip the outer margin: some radars (e.g. nuke lower) have a frame line around the image
    for (let y = EDGE; y < N - EDGE; y++)
      for (let x = EDGE; x < N - EDGE; x++)
        if (isFloor(img.pixels, y * N + x)) (rows[y]++, cols[x]++)
  }
  const cut = (a: number[]) => Math.max(6, Math.max(...a) * 0.08)
  const cx = cut(cols), cy = cut(rows)
  const x0 = cols.findIndex((c) => c >= cx), x1 = cols.findLastIndex((c) => c >= cx)
  const y0 = rows.findIndex((c) => c >= cy), y1 = rows.findLastIndex((c) => c >= cy)
  if (x0 < 0 || y0 < 0) return { x: 0, y: 0, w: 1024, h: 1024 }
  const k = 1024 / N
  return { x: (x0 - 1) * k, y: (y0 - 1) * k, w: (x1 - x0 + 3) * k, h: (y1 - y0 + 3) * k }
}

// Renders the cropped radar to fit maxW x maxH dots, keeping aspect ratio.
export function rasterize(png: Buffer, ov: Overview, box: Box, maxW: number, maxH: number): Radar {
  const w = Math.max(8, Math.min(maxW, Math.floor((maxH * box.w) / box.h)))
  const img = new Resvg(svgOf(png, box), { fitTo: { mode: 'width', value: w * SS } }).render()
  const k = img.width / SS / box.w // dots per radar pixel
  return {
    width: Math.floor(img.width / SS),
    height: Math.floor(img.height / SS),
    ss: SS,
    pixels: img.pixels,
    toPx: (x, y) => [((x - ov.posX) / ov.scale - box.x) * k, ((ov.posY - y) / ov.scale - box.y) * k],
    normPx: (nx, ny) => [(nx * 1024 - box.x) * k, (ny * 1024 - box.y) * k],
  }
}
