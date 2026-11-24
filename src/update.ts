import { chmodSync, renameSync, rmSync, writeFileSync } from 'node:fs'
import { basename } from 'node:path'
import { gunzipSync } from 'node:zlib'

export const REPO = 'dormonbear/gotv-tui'

// Release asset for this platform, e.g. gotv-tui-darwin-arm64, gotv-tui-windows-x64.exe
export function assetName(platform: string = process.platform, arch: string = process.arch): string | null {
  const os = { darwin: 'darwin', linux: 'linux', win32: 'windows' }[platform]
  if (!os || (arch !== 'x64' && arch !== 'arm64')) return null
  return `gotv-tui-${os}-${arch}${os === 'windows' ? '.exe' : ''}`
}

// True when running as the compiled single-file binary (not via node/tsx/bun).
export const isBinary = () => /^gotv-tui/.test(basename(process.execPath))

export async function selfUpdate(current: string): Promise<string> {
  if (!isBinary()) return 'Running from source: update with git pull && npm install.'
  const asset = assetName()
  if (!asset) return `No prebuilt binary for ${process.platform}-${process.arch}.`
  const r = await fetch(`https://api.github.com/repos/${REPO}/releases/latest`, {
    headers: { accept: 'application/vnd.github+json', 'user-agent': 'gotv-tui' },
  })
  if (!r.ok) throw new Error(`GitHub API: HTTP ${r.status}`)
  const rel = (await r.json()) as { tag_name: string; assets: { name: string; browser_download_url: string }[] }
  const latest = rel.tag_name.replace(/^v/, '')
  if (latest === current) return `Already up to date (v${current}).`
  const url = rel.assets.find((a) => a.name === `${asset}.gz`)?.browser_download_url
  if (!url) throw new Error(`release ${rel.tag_name} has no ${asset}.gz`)
  const bin = await fetch(url, { headers: { 'user-agent': 'gotv-tui' } })
  if (!bin.ok) throw new Error(`download: HTTP ${bin.status}`)
  const target = process.execPath
  const tmp = `${target}.new`
  writeFileSync(tmp, gunzipSync(Buffer.from(await bin.arrayBuffer())))
  chmodSync(tmp, 0o755)
  // a running .exe can't be overwritten on Windows, but it can be renamed away
  if (process.platform === 'win32') {
    rmSync(`${target}.old`, { force: true })
    renameSync(target, `${target}.old`)
  }
  renameSync(tmp, target)
  return `Updated v${current} -> v${latest}.`
}
