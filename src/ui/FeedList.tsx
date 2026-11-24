import { Box, Text, useInput } from 'ink'
import Spinner from 'ink-spinner'
import TextInput from 'ink-text-input'
import { useEffect, useState } from 'react'
import { FEEDS_FILE, isRelayUrl, loadFeeds, normalizeUrl, probeFeed, saveFeeds, type Feed, type FeedStatus } from '../feeds.ts'

type Form = { index: number | null; name: string; url: string; field: 'name' | 'url'; error: string }
const STATE_COLOR = { live: 'green', stale: 'yellow', waiting: 'cyan', offline: 'gray' } as const

type Props = { onOpen: (url: string) => void; onEditing: (editing: boolean) => void }

export function FeedList({ onOpen, onEditing }: Props) {
  const [feeds, setFeeds] = useState<Feed[]>(loadFeeds)
  const [status, setStatus] = useState<Map<string, FeedStatus>>(new Map())
  const [sel, setSel] = useState(0)
  const [form, setForm] = useState<Form | null>(null)
  const [confirmDelete, setConfirmDelete] = useState(false)

  // clear the probed rows first so they show a spinner until their answer arrives
  const probe = (list: Feed[]) => {
    setStatus((m) => {
      const next = new Map(m)
      list.forEach((f) => next.delete(f.url))
      return next
    })
    list.forEach((f) => probeFeed(f).then((s) => setStatus((m) => new Map(m).set(f.url, s))))
  }
  const probing = feeds.some((f) => !status.has(f.url))
  useEffect(() => probe(feeds), [])
  useEffect(() => onEditing(form !== null), [form !== null])

  const commit = (next: Feed[]) => {
    setFeeds(next)
    saveFeeds(next)
    setSel((i) => Math.max(0, Math.min(i, next.length - 1)))
  }

  useInput(
    (input, key) => {
      if (confirmDelete) {
        if (input === 'y') commit(feeds.filter((_, i) => i !== sel))
        setConfirmDelete(false)
        return
      }
      if (key.upArrow) setSel((i) => Math.max(0, i - 1))
      if (key.downArrow) setSel((i) => Math.min(feeds.length - 1, i + 1))
      if (key.return && feeds[sel]) onOpen(feeds[sel].url)
      if (input === 'r') probe(feeds)
      if (input === 'a') setForm({ index: null, name: '', url: '', field: 'name', error: '' })
      if (input === 'e' && feeds[sel]) setForm({ index: sel, ...feeds[sel], field: 'name', error: '' })
      if (input === 'd' && feeds[sel]) setConfirmDelete(true)
    },
    { isActive: form === null },
  )

  useInput(
    (_, key) => {
      if (key.escape) setForm(null)
      if (key.tab) setForm((f) => f && { ...f, field: f.field === 'name' ? 'url' : 'name' })
    },
    { isActive: form !== null },
  )

  const submit = () => {
    if (!form) return
    if (form.field === 'name') return setForm({ ...form, field: 'url' })
    const url = normalizeUrl(form.url)
    if (!isRelayUrl(url)) return setForm({ ...form, error: 'URL must start with http(s)://' })
    const feed = { name: form.name.trim() || url, url }
    const next = form.index === null ? [...feeds, feed] : feeds.map((f, i) => (i === form.index ? feed : f))
    commit(next)
    if (form.index === null) setSel(next.length - 1)
    setForm(null)
    probe([feed])
  }

  return (
    <Box flexDirection="column">
      <Text bold>
        CSTV / GOTV sources{' '}
        {probing && (
          <Text color="cyan">
            <Spinner type="dots" /> refreshing
          </Text>
        )}
      </Text>
      {feeds.length === 0 && <Text dimColor>No sources. Press a to add one.</Text>}
      {feeds.map((f, i) => {
        const s = status.get(f.url)
        return (
          <Text key={i} inverse={i === sel && !form} wrap="truncate">
            {` ${f.name}  `}
            {s ? (
              <Text color={STATE_COLOR[s.state]}>{s.state === 'offline' ? 'offline' : s.state === 'waiting' ? 'waiting for broadcast' : `${s.state} · ${s.map}`}</Text>
            ) : (
              <Text color="cyan">
                <Spinner type="dots" /> probing
              </Text>
            )}
            <Text dimColor>{`  ${f.url}`}</Text>
          </Text>
        )
      })}
      {form && (
        <Box flexDirection="column" marginTop={1} borderStyle="round" paddingX={1}>
          <Text bold>{form.index === null ? 'Add source' : 'Edit source'}</Text>
          <Box>
            <Text>{form.field === 'name' ? '› ' : '  '}Name: </Text>
            <TextInput value={form.name} focus={form.field === 'name'} onChange={(name) => setForm({ ...form, name })} onSubmit={submit} />
          </Box>
          <Box>
            <Text>{form.field === 'url' ? '› ' : '  '}URL:  </Text>
            <TextInput
              value={form.url}
              focus={form.field === 'url'}
              placeholder='https://... or playcast "https://..."'
              onChange={(url) => setForm({ ...form, url, error: '' })}
              onSubmit={submit}
            />
          </Box>
          {form.error && <Text color="red">{form.error}</Text>}
          <Text dimColor>enter next/save · tab switch field · esc cancel</Text>
        </Box>
      )}
      {confirmDelete && <Text color="yellow">{`Delete "${feeds[sel]?.name}"? y to confirm, any other key cancels`}</Text>}
      {!form && (
        <Text dimColor>
          ↑/↓ select · enter open · a add · e edit · d delete · r refresh · q quit
        </Text>
      )}
      <Text dimColor>{`sources saved in ${FEEDS_FILE}`}</Text>
    </Box>
  )
}
