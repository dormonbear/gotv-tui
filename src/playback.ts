// CSTV fragments arrive once a second and their ~64 ticks are parsed in a burst.
// Playback re-times them: samples are queued by tick and released at wall-clock
// pace, `bufferS` behind the newest tick, so the UI moves smoothly instead of
// jumping once per second.
export class Playback<S, E> {
  private frames: { tick: number; s: S }[] = []
  private events: { tick: number; e: E }[] = []
  private play = -1
  private wall = 0

  constructor(
    private tps: number,
    private bufferS = 1.5,
    private resyncS = 5, // fell this far behind (stall, catch-up burst): jump instead of crawling
  ) {}

  pushFrame(tick: number, s: S) {
    this.frames.push({ tick, s })
  }

  pushEvent(tick: number, e: E) {
    this.events.push({ tick, e })
  }

  // Advance the playback clock to wall time `now` (ms); returns what became due.
  step(now: number): { frame: S | null; events: E[] } {
    const newest = Math.max(this.frames.at(-1)?.tick ?? -1, this.events.at(-1)?.tick ?? -1)
    if (newest < 0) return { frame: null, events: [] }
    if (this.play < 0 || newest - this.play > this.tps * this.resyncS) this.play = newest - this.tps * this.bufferS
    else this.play = Math.min(newest, this.play + ((now - this.wall) / 1000) * this.tps)
    this.wall = now
    let frame: S | null = null
    while (this.frames.length && this.frames[0].tick <= this.play) frame = this.frames.shift()!.s
    const events: E[] = []
    while (this.events.length && this.events[0].tick <= this.play) events.push(this.events.shift()!.e)
    return { frame, events }
  }
}
