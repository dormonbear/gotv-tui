#!/usr/bin/env -S npx tsx
import { render } from 'ink'
import pkg from '../package.json' with { type: 'json' }
import { normalizeUrl } from './feeds.ts'
import { App } from './ui/App.tsx'
import { selfUpdate } from './update.ts'

const USAGE = `gotv-tui ${pkg.version} - live CS2 matches from CSTV relays

usage:
  gotv-tui                      pick a source from the list
  gotv-tui <relay url>          open a relay, e.g. https://cstv.fragnet.net/a
  gotv-tui 'playcast "<url>"'   same, pasted from a tournament page
  gotv-tui update               update the binary to the latest release
  gotv-tui --version`

const arg = process.argv.slice(2).join(' ').trim()
if (arg === '--version' || arg === '-v') {
  console.log(pkg.version)
  process.exit(0)
}
if (arg === 'update') {
  try {
    console.log(await selfUpdate(pkg.version))
    process.exit(0)
  } catch (e) {
    console.error(`update failed: ${(e as Error).message}`)
    process.exit(1)
  }
}
if (arg && !/https?:\/\//.test(arg)) {
  console.log(USAGE)
  process.exit(arg === '--help' || arg === '-h' ? 0 : 1)
}

process.stdout.write('\x1b[?1049h') // alternate screen
const app = render(<App url={arg ? normalizeUrl(arg) : undefined} />, { exitOnCtrlC: true })
await app.waitUntilExit()
process.stdout.write('\x1b[?1049l')
process.exit(0)
