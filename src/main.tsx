#!/usr/bin/env -S npx tsx
import { render } from 'ink'
import { normalizeUrl } from './feeds.ts'
import { App } from './ui/App.tsx'

const arg = process.argv.slice(2).join(' ').trim()
if (arg && !/https?:\/\//.test(arg)) {
  console.log('usage: gotv-tui [<cstv relay url> | playcast "<url>"]')
  process.exit(1)
}

process.stdout.write('\x1b[?1049h') // alternate screen
const app = render(<App url={arg ? normalizeUrl(arg) : undefined} />, { exitOnCtrlC: true })
await app.waitUntilExit()
process.stdout.write('\x1b[?1049l')
process.exit(0)
