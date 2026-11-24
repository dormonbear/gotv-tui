import { Box, Text } from 'ink'
import { bg, fg, RESET, sideColor } from '../format.ts'
import type { Nade, PlayerState, Side } from '../gotv.ts'

const NADE_GLYPH: Record<Nade, string> = {
  he: '\x1b[38;2;235;80;70mH',
  flash: '\x1b[38;2;240;240;240mF',
  smoke: '\x1b[38;2;160;165;175mS',
  fire: '\x1b[38;2;240;130;40mM',
  decoy: '\x1b[38;2;120;200;120mD',
}
const nadeText = (n: Nade[]) => (n.length ? '\x1b[1m' + n.map((x) => NADE_GLYPH[x]).join('') + RESET : '')
// Header columns line up with the row layout below; the team title fills the first 17.
const TITLE_W = 17
const COLUMNS = 'hp'.padEnd(14) + ' ' + '$'.padStart(6) + '   K  D  A' + 'ADR'.padStart(6) + '  kit ' + 'weapon'.padEnd(11) + 'util'
const title = (side: Side, t: { name: string; score: number }) => {
  const tail = ` ${t.score}`
  return (` ${side} ` + t.name).slice(0, TITLE_W - 1 - tail.length).concat(tail).padEnd(TITLE_W) // keep one space before "hp"
}
const gunName = (g: string | null) => (g ?? '').replace('_silencer', '-s').replace('_', '-').slice(0, 10)

const hpBar = (hp: number) => {
  const n = Math.round(hp / 10)
  const c = hp > 60 ? '\x1b[32m' : hp > 25 ? '\x1b[33m' : '\x1b[31m'
  return c + '█'.repeat(n) + '\x1b[2m' + '░'.repeat(10 - n) + RESET
}

// Stable 1..5 numbering per side, shared by the table badges and the minimap dots.
export function playerNumbers(players: PlayerState[]): Map<string, number> {
  const m = new Map<string, number>()
  for (const s of ['CT', 'T'] as const)
    players
      .filter((p) => p.side === s)
      .map((p) => p.name.toLowerCase())
      .sort()
      .forEach((n, i) => m.set(n, i + 1))
  return m
}

type Props = { side: Side; team: { name: string; score: number }; players: PlayerState[]; nums: Map<string, number>; width: number }

export function TeamTable({ side, team, players, nums, width }: Props) {
  const color = sideColor(side)
  const rows = players
    .filter((p) => p.side === side)
    .sort((a, b) => (nums.get(a.name.toLowerCase()) ?? 9) - (nums.get(b.name.toLowerCase()) ?? 9))
  return (
    <Box flexDirection="column" width={width}>
      <Text wrap="truncate">
        {`${fg(color)}\x1b[1m${title(side, team)}${RESET}`}
        {'\x1b[2m' + COLUMNS + RESET}
      </Text>
      {rows.map((p) => {
        const badge = bg(color) + fg([10, 10, 10]) + ` ${nums.get(p.name.toLowerCase()) ?? ' '} ` + RESET
        const kit = (p.helmet ? 'H' : p.armor > 0 ? 'A' : ' ') + (p.defuser ? 'K' : ' ') + (p.hasBomb ? '\x1b[31mB' + RESET : ' ')
        const row =
          `${badge} ${p.name.padEnd(12).slice(0, 12)} ${p.alive ? hpBar(p.hp) + String(p.hp).padStart(4) : '\x1b[2m   dead       ' + RESET}` +
          ` ${('$' + p.money).padStart(6)} ${String(p.kills).padStart(3)}${String(p.deaths).padStart(3)}${String(p.assists).padStart(3)}` +
          ` ${p.adr.toFixed(0).padStart(5)}  ${kit} ${p.alive ? gunName(p.gun).padEnd(10) + ' ' + nadeText(p.nades) : ''}`
        return (
          <Text key={p.name} wrap="truncate" dimColor={!p.alive}>
            {row}
          </Text>
        )
      })}
    </Box>
  )
}
