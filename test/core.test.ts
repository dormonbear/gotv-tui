import assert from 'node:assert/strict'
import { test } from 'node:test'
import { mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { feedState, isRelayUrl, loadFeeds, normalizeUrl, PRESETS, saveFeeds } from '../src/feeds.ts'
import { formatEvent } from '../src/format.ts'
import { entityOrigin, loadout, type MatchState, type PlayerState } from '../src/gotv.ts'
import { parseOverview, resample, sitesOn, type Radar } from '../src/maps.ts'
import { buildBase, drawMinimap, facing, levelOf } from '../src/minimap.ts'
import { Playback } from '../src/playback.ts'
import { assetName } from '../src/update.ts'
import { applyEvent, type MatchData } from '../src/ui/useMatch.ts'

const strip = (s: string) => s.replace(/\x1b\[[0-9;]*m/g, '')

test('parseOverview reads calibration and the lower-level split', () => {
  const nuke = `"de_nuke" { "pos_x" "-3453" "pos_y" "2887" "scale" "7"
    "verticalsections" { "default" { "AltitudeMax" "10000" "AltitudeMin" "-495" }
    "lower" { "AltitudeMax" "-495" "AltitudeMin" "-10000" } }
    "bombA_x" "0.58" "bombA_y" "0.48" "bombB_x" "0.58" "bombB_y" "0.58" }`
  assert.deepEqual(parseOverview(nuke), {
    posX: -3453, posY: 2887, scale: 7, lowerBelow: -495,
    sites: [{ name: 'A', x: 0.58, y: 0.48 }, { name: 'B', x: 0.58, y: 0.58 }],
  })
  assert.equal(parseOverview('"pos_x" "1" "pos_y" "2" "scale" "4.4"').lowerBelow, null)
})

test('entityOrigin decodes cell + offset coordinates', () => {
  const pre = 'CSmokeGrenadeProjectile.CBodyComponentBaseAnimGraph.m_skeletonInstance.m_vecOrigin.'
  const props = { [pre + 'm_cellX']: 34, [pre + 'm_cellY']: 28, [pre + 'm_cellZ']: 30, [pre + 'm_vecX']: 118.5, [pre + 'm_vecY']: 20, [pre + 'm_vecZ']: 610 }
  assert.deepEqual(entityOrigin(props), { x: 34 * 512 - 16384 + 118.5, y: 28 * 512 - 16384 + 20, z: -414 })
})

test('kill line shows assist, flash and tags', () => {
  const line = formatEvent({
    type: 'kill', killer: { name: 'K', side: 'CT' }, victim: { name: 'V', side: 'T' }, assister: { name: 'H', side: 'CT', flash: true },
    weapon: 'ak47', headshot: true, wallbang: false, smoke: true, noscope: false, blind: false, from: null, to: null,
  })
  assert.equal(strip(line), 'K + H(flash) [ak47 HS smoke] V')
})

test('self or world kill reads as a death', () => {
  const v = { name: 'V', side: 'CT' as const }
  const base = { assister: null, headshot: false, wallbang: false, smoke: false, noscope: false, blind: false, from: null, to: null }
  assert.equal(strip(formatEvent({ type: 'kill', killer: v, victim: v, weapon: 'world', ...base })), 'V died [world]')
  assert.equal(strip(formatEvent({ type: 'kill', killer: null, victim: v, weapon: 'inferno', ...base })), 'V died [inferno]')
})

test('applyEvent keeps kill marks for the round and clears them on round start', () => {
  let d: MatchData = { st: null, log: [], kills: [], status: '', radar: null }
  const to = { x: 1, y: 2, z: 3 }
  d = applyEvent(d, { type: 'kill', killer: null, victim: { name: 'V', side: 'T' }, assister: null, weapon: 'awp', headshot: false, wallbang: false, smoke: false, noscope: false, blind: false, from: null, to }, 5)
  assert.deepEqual(d.kills, [{ from: null, to, victim: 'T', at: 5 }])
  d = applyEvent(d, { type: 'roundStart', round: 2 }, 6)
  assert.equal(d.kills.length, 0)
  assert.equal(d.log[0].type, 'roundStart')
})

test('drawMinimap places players by braille cell and draws only its own level', () => {
  const W = 20, H = 20
  const radar: Radar = { width: W, height: H, ss: 1, pixels: new Uint8Array(W * H * 4).fill(255), toPx: (x, y) => [x, y], normPx: (nx, ny) => [nx * W, ny * H] }
  const pl = (name: string, x: number, y: number, z: number): PlayerState => ({
    name, side: 'T', pos: { x, y, z }, yaw: 0, alive: true, hp: 100, armor: 0, helmet: false, defuser: false, money: 0, kills: 0, deaths: 0, assists: 0, adr: 0, gun: null, nades: [], hasBomb: false,
  })
  const st = { players: [pl('a', 7, 9, 0), pl('b', 1, 1, -600)], utility: [], bomb: null } as unknown as MatchState
  const base = buildBase(radar)
  const rows = drawMinimap(radar, base, st, { kills: [], nums: new Map([['a', 2], ['b', 3]]), now: 0, level: 'upper', lowerBelow: -495, sites: [{ name: 'A', x: 0.9, y: 0.9 }] })
  assert.equal(rows.length, 5)
  assert.equal(strip(rows[2])[3], '2') // x=7 -> col 3, y=9 -> row 2
  assert.equal(strip(rows[2])[4], '→') // yaw 0 = facing east, arrow in the next cell
  assert.notEqual(strip(rows[0])[0], '3') // lower-level player is not on the upper map
  assert.equal(strip(rows[4])[9], 'A') // site letter at 0.9, 0.9 -> col 9, row 4
  const lower = drawMinimap(radar, base, st, { kills: [], nums: new Map([['a', 2], ['b', 3]]), now: 0, level: 'lower', lowerBelow: -495, sites: [] })
  assert.equal(strip(lower[0])[0], '3') // ...it is on the lower map
  assert.notEqual(strip(lower[2])[3], '2')
  assert.equal(levelOf(-600, -495), 'lower')
  assert.equal(levelOf(-600, null), 'upper')
})

test('sitesOn puts nuke B on the lower level, everything else upper', () => {
  const sites = [{ name: 'A', x: 0, y: 0 }, { name: 'B', x: 0, y: 0 }]
  assert.deepEqual(sitesOn('de_nuke', 'upper', sites).map((s) => s.name), ['A'])
  assert.deepEqual(sitesOn('de_nuke', 'lower', sites).map((s) => s.name), ['B'])
  assert.deepEqual(sitesOn('de_train', 'upper', sites).map((s) => s.name), ['A', 'B'])
})

test('buildBase keeps a 1px wall that a plain downscale would blur away', () => {
  // 16x16 image, ss=4 -> 4x4 dots (2x1 cells); a 1px transparent column at x=6 splits two rooms
  const IW = 16, IH = 16, px = new Uint8Array(IW * IH * 4)
  for (let y = 0; y < IH; y++)
    for (let x = 0; x < IW; x++) {
      const i = (y * IW + x) * 4
      if (x !== 6) px.set([80, 100, 110, 255], i)
    }
  const r: Radar = { width: 4, height: 4, ss: 4, pixels: px, toPx: (x, y) => [x, y], normPx: (x, y) => [x, y] }
  const b = buildBase(r)
  assert.equal(b.CW, 2)
  assert.ok(b.dots[0] & 0x8 && b.dots[1] & 0x1, 'dots light up on both sides of the wall')
  assert.ok(b.bg[0] && b.bg[1], 'both cells are floor')
})

test('loadout picks the primary gun and lists grenades in a fixed order', () => {
  const it = (name: string, quantity = 1) => ({ name, quantity })
  assert.deepEqual(loadout([it('knife'), it('glock'), it('ak47'), it('flashbang', 2), it('hegrenade'), it('c4')]), { gun: 'ak47', nades: ['he', 'flash', 'flash'] })
  assert.deepEqual(loadout([it('knife'), it('deagle'), it('incgrenade'), it('smokegrenade')]), { gun: 'deagle', nades: ['smoke', 'fire'] })
  assert.deepEqual(loadout([it('knife')]), { gun: null, nades: [] })
})

test('resample averages by alpha and maps the 1024-space box onto the image', () => {
  // 2x2 image: left column opaque red, right column transparent
  const data = new Uint8Array([255, 0, 0, 255, 0, 0, 0, 0, 255, 0, 0, 255, 0, 0, 0, 0])
  const img = { width: 2, height: 2, data }
  assert.deepEqual([...resample(img, { x: 0, y: 0, w: 1024, h: 1024 }, 1, 1)], [255, 0, 0, 127]) // colour kept, half coverage (127.5 truncated)
  assert.deepEqual([...resample(img, { x: 512, y: 0, w: 512, h: 1024 }, 1, 1)], [0, 0, 0, 0]) // right half only
})

test('facing maps yaw to an arrow and screen offset (north is up)', () => {
  assert.deepEqual(facing(0), { ch: '→', dx: 1, dy: -0 })
  assert.deepEqual(facing(90), { ch: '↑', dx: 0, dy: -1 })
  assert.deepEqual(facing(-135), { ch: '↙', dx: -1, dy: 1 })
  assert.equal(facing(359).ch, '→')
})

test('normalizeUrl accepts a pasted playcast command', () => {
  assert.equal(normalizeUrl(' playcast "https://cstv.fragnet.net/a/" '), 'https://cstv.fragnet.net/a')
  assert.ok(isRelayUrl('playcast "https://x.y/a"'))
  assert.ok(!isRelayUrl('cstv.fragnet.net/a'))
})

test('feeds persist to a file and fall back to presets', () => {
  const file = join(mkdtempSync(join(tmpdir(), 'gotv-')), 'feeds.json')
  assert.deepEqual(loadFeeds(file), PRESETS) // missing file
  saveFeeds([{ name: 'Mine', url: 'https://r.example/x' }], file)
  assert.deepEqual(loadFeeds(file), [{ name: 'Mine', url: 'https://r.example/x' }])
  writeFileSync(file, '{broken')
  assert.deepEqual(loadFeeds(file), PRESETS)
})

test('feedState tells live, stale and offline apart', () => {
  assert.equal(feedState({ tick: 1, rcvage: 0.2 }), 'live')
  assert.equal(feedState({ tick: 1, rcvage: 367 }), 'stale')
  assert.equal(feedState(null), 'offline')
})

test('Playback releases a burst smoothly at wall-clock pace', () => {
  const pb = new Playback<number, string>(64, 1.5, 5)
  for (let t = 0; t <= 256; t += 4) pb.pushFrame(t, t) // one burst = 4 s of game time
  pb.pushEvent(170, 'kill')
  assert.deepEqual(pb.step(0), { frame: 160, events: [] }) // starts 1.5 s (96 ticks) behind newest
  assert.deepEqual(pb.step(62.5), { frame: 164, events: [] }) // +4 ticks per 1/16 s
  assert.deepEqual(pb.step(125), { frame: 168, events: [] })
  assert.deepEqual(pb.step(187.5), { frame: 172, events: ['kill'] })
  assert.deepEqual(pb.step(187.5), { frame: null, events: [] }) // nothing new due
  pb.pushFrame(1000, 1000) // far ahead (stall/catch-up): jump to 1000 - 96 = 904
  assert.equal(pb.step(250).frame, 256) // stale queue flushed in one go, no slow crawl
  pb.pushFrame(1004, 1004)
  assert.equal(pb.step(312.5).frame, null) // play is now ~908: 1000 not due yet
})

test('assetName maps platform/arch to release asset names', () => {
  assert.equal(assetName('darwin', 'arm64'), 'gotv-tui-darwin-arm64')
  assert.equal(assetName('linux', 'x64'), 'gotv-tui-linux-x64')
  assert.equal(assetName('win32', 'x64'), 'gotv-tui-windows-x64.exe')
  assert.equal(assetName('freebsd', 'x64'), null)
  assert.equal(assetName('linux', 'ia32'), null)
})
