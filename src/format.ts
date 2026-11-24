import type { LogEvent, Side } from './gotv.ts'

// Game-log lines as ANSI strings (Ink's <Text> renders ANSI as-is).

export type RGB = [number, number, number]
export const CT: RGB = [90, 170, 255]
export const T: RGB = [240, 176, 48]

export const fg = ([r, g, b]: RGB) => `\x1b[38;2;${r};${g};${b}m`
export const bg = ([r, g, b]: RGB) => `\x1b[48;2;${r};${g};${b}m`
export const RESET = '\x1b[0m'
const dim = (s: string) => `\x1b[2m${s}${RESET}`
const bold = (s: string) => `\x1b[1m${s}${RESET}`

export const sideColor = (s: Side): RGB => (s === 'CT' ? CT : T)
const name = (w: { name: string; side: Side } | null) => (w ? `${fg(sideColor(w.side))}${w.name}${RESET}` : '?')

export function formatEvent(e: LogEvent): string {
  switch (e.type) {
    case 'kill': {
      if (!e.killer || e.killer.name === e.victim.name) return `${name(e.victim)} ${dim(`died [${e.weapon}]`)}`
      const who = name(e.killer) + (e.assister ? ` + ${name(e.assister)}${e.assister.flash ? dim('(flash)') : ''}` : '')
      const tags = [e.weapon, e.headshot && 'HS', e.wallbang && 'wall', e.smoke && 'smoke', e.noscope && 'noscope', e.blind && 'blind']
        .filter(Boolean)
        .join(' ')
      return `${who} ${dim('[' + tags + ']')} ${name(e.victim)}`
    }
    case 'roundStart':
      return bold(`── Round ${e.round} started`)
    case 'roundEnd':
      return bold(`── Round over - ${e.winner ? fg(sideColor(e.winner)) + e.winner + RESET + '\x1b[1m' : '?'} ${e.reason}`)
    case 'plant':
      return `${fg(T)}${e.player ?? '?'}${RESET} planted the bomb`
    case 'defuse':
      return `${fg(CT)}${e.player ?? '?'}${RESET} defused the bomb`
    case 'explode':
      return bold('Bomb exploded')
  }
}
