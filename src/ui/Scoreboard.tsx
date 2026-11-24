import { Box, Text } from 'ink'
import { bg, fg, RESET, sideColor } from '../format.ts'
import type { PlayerState, Side } from '../gotv.ts'

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
        {`${fg(color)}\x1b[1m ${side} ${team.name} ${team.score}${RESET}`}
        {'\x1b[2m    hp              $      K  D  A   ADR  weapon' + RESET}
      </Text>
      {rows.map((p) => {
        const badge = bg(color) + fg([10, 10, 10]) + ` ${nums.get(p.name.toLowerCase()) ?? ' '} ` + RESET
        const kit = (p.helmet ? 'H' : p.armor > 0 ? 'A' : ' ') + (p.defuser ? 'K' : ' ') + (p.hasBomb ? '\x1b[31mB' + RESET : ' ')
        const row =
          `${badge} ${p.name.padEnd(12).slice(0, 12)} ${p.alive ? hpBar(p.hp) + String(p.hp).padStart(4) : '\x1b[2m   dead       ' + RESET}` +
          ` ${('$' + p.money).padStart(6)} ${String(p.kills).padStart(3)}${String(p.deaths).padStart(3)}${String(p.assists).padStart(3)}` +
          ` ${p.adr.toFixed(0).padStart(5)}  ${kit} ${p.alive ? (p.weapon ?? '') : ''}`
        return (
          <Text key={p.name} wrap="truncate" dimColor={!p.alive}>
            {row}
          </Text>
        )
      })}
    </Box>
  )
}
