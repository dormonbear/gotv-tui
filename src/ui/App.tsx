import { useApp, useInput, useStdout } from 'ink'
import { useEffect, useState } from 'react'
import { FeedList } from './FeedList.tsx'
import { MatchView } from './MatchView.tsx'

function useSize() {
  const { stdout } = useStdout()
  const read = () => ({ cols: stdout.columns || 100, rows: stdout.rows || 40 })
  const [size, setSize] = useState(read)
  useEffect(() => {
    const f = () => setSize(read())
    stdout.on('resize', f)
    return () => void stdout.off('resize', f)
  }, [stdout])
  return size
}

export function App({ url }: { url?: string }) {
  const { exit } = useApp()
  const { cols, rows } = useSize()
  const [open, setOpen] = useState<string | null>(url ?? null)
  const [editing, setEditing] = useState(false)
  useInput(
    (input) => {
      if (input === 'q') exit()
      if (input === 'b' && open) setOpen(null)
    },
    { isActive: !editing },
  )
  return open ? (
    <MatchView key={open} url={open} cols={cols} rows={rows} />
  ) : (
    <FeedList onOpen={setOpen} onEditing={setEditing} />
  )
}
