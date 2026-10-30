import {useEffect, useRef, useState} from 'react'
import {CheckIcon, CircleIcon, Loader2Icon, XIcon} from 'lucide-react'
import {cn} from '@/lib/utils'
import {CopyPlan, CopyProgress as Progress, Phase} from '@/lib/api'

export type RunStatus = 'idle' | 'running' | 'success' | 'failed'

const streamingSteps: {phase: Phase; label: string}[] = [
  {phase: 'connect', label: 'Connect'},
  {phase: 'erase', label: 'Erase target'},
  {phase: 'schema', label: 'Schema'},
  {phase: 'data', label: 'Data'},
  {phase: 'finalize', label: 'Indexes & constraints'},
  {phase: 'done', label: 'Done'},
]

// Parallel mode dumps to disk before touching the target.
const parallelSteps = [streamingSteps[0], {phase: 'dump' as Phase, label: 'Dump to disk'}, ...streamingSteps.slice(1)]

const MAX_TABLE_ROWS = 300

// Share of the overall bar each phase starts at; dump, data and finalize are
// filled proportionally, the short phases just jump ahead.
const streamingStart: Record<Phase, number> = {connect: 0, dump: 0, erase: 2, schema: 5, data: 10, finalize: 85, done: 100}
const parallelStart: Record<Phase, number> = {connect: 0, dump: 2, erase: 40, schema: 42, data: 45, finalize: 85, done: 100}

function stageFraction(p: Progress): number {
  const frac = p.bytesTotal > 0 ? p.bytesDone / p.bytesTotal : p.tablesTotal > 0 ? p.tablesDone / p.tablesTotal : 0
  return Math.min(frac, 1)
}

export function overallPercent(p: Progress | null): number {
  if (!p) return 0
  const start = p.parallel ? parallelStart : streamingStart
  const span = (ph: Phase, next: Phase) => start[next] - start[ph]
  switch (p.phase) {
    case 'dump':
      return start.dump + span('dump', 'erase') * stageFraction(p)
    case 'schema':
      return start.schema + span('schema', 'data') * Math.min(p.schemaObjects / (p.tablesTotal * 3 + 10), 0.99)
    case 'data':
      return start.data + span('data', 'finalize') * stageFraction(p)
    case 'finalize':
      return start.finalize + span('finalize', 'done') * (p.objectsTotal > 0 ? Math.min(p.objectsDone / p.objectsTotal, 0.99) : 0)
    default:
      return start[p.phase]
  }
}

export function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  const units = ['KiB', 'MiB', 'GiB', 'TiB']
  let v = n / 1024
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(v < 10 ? 1 : 0)} ${units[i]}`
}

export function formatDuration(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000))
  const h = Math.floor(s / 3600)
  const m = Math.floor((s % 3600) / 60)
  const sec = s % 60
  return h > 0 ? `${h}h ${m}m` : m > 0 ? `${m}m ${sec}s` : `${sec}s`
}

// Throughput over the last few seconds of a byte counter; resets when it drops
// (a new stage started).
function useRate(bytes: number, active: boolean): number {
  const samples = useRef<{t: number; b: number}[]>([])
  const [rate, setRate] = useState(0)
  useEffect(() => {
    if (!active) {
      samples.current = []
      setRate(0)
      return
    }
    const now = Date.now()
    const s = samples.current
    if (s.length > 0 && bytes < s[s.length - 1].b) s.length = 0
    s.push({t: now, b: bytes})
    while (s.length > 1 && now - s[0].t > 5000) s.shift()
    const first = s[0]
    setRate(now > first.t ? ((bytes - first.b) * 1000) / (now - first.t) : 0)
  }, [bytes, active])
  return rate
}

function useNow(active: boolean): number {
  const [now, setNow] = useState(Date.now())
  useEffect(() => {
    if (!active) return
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [active])
  return now
}

export function CopyProgress({status, plan, progress, doneTables, startedAt, finishedAt, error}: {
  status: RunStatus
  plan: CopyPlan | null
  progress: Progress | null
  doneTables: Set<string>
  startedAt: number | null
  finishedAt: number | null
  error: string
}) {
  const running = status === 'running'
  const now = useNow(running)
  const parallel = progress?.parallel ?? false
  const rate = useRate(progress?.bytesDone ?? 0, running)

  if (status === 'idle' || !startedAt) {
    return <div className="flex h-full min-h-48 items-center justify-center text-sm text-muted-foreground">Nothing yet.</div>
  }

  const phase: Phase = progress?.phase ?? 'connect'
  const percent = status === 'success' ? 100 : overallPercent(progress)
  const elapsed = (finishedAt ?? now) - startedAt
  const remainingBytes = progress ? progress.bytesTotal - progress.bytesDone : 0
  const transferring = phase === 'data' || phase === 'dump'
  const eta = running && transferring && rate > 0 && remainingBytes > 0 ? remainingBytes / rate * 1000 : null
  const steps = parallel ? parallelSteps : streamingSteps
  const activeIndex = status === 'success' ? steps.length - 1 : steps.findIndex(s => s.phase === phase)

  return (
    <div className="grid gap-5">
      <div className="grid gap-2">
        <div className="flex items-baseline justify-between">
          <div className="text-3xl font-semibold tabular-nums">{Math.floor(percent)}%</div>
          <div className="text-sm text-muted-foreground tabular-nums">
            {formatDuration(elapsed)} elapsed
            {eta !== null && <> · ~{formatDuration(eta)} left{parallel && (phase === 'dump' ? ' in dump' : ' in restore')}</>}
          </div>
        </div>
        <div className="h-2.5 overflow-hidden rounded-full bg-muted">
          <div className={cn('h-full rounded-full transition-[width] duration-300',
            status === 'failed' ? 'bg-destructive' : status === 'success' ? 'bg-success' : 'bg-primary')}
               style={{width: `${percent}%`}}/>
        </div>
      </div>

      <ol className="grid gap-2" style={{gridTemplateColumns: `repeat(${steps.length}, minmax(0, 1fr))`}}>
        {steps.map((s, i) => {
          const state = i < activeIndex || status === 'success' ? 'done'
            : i === activeIndex ? (status === 'failed' ? 'failed' : 'active') : 'pending'
          return (
            <li key={s.phase} className="flex flex-col items-center gap-1.5 text-center">
              <div className={cn('flex size-7 items-center justify-center rounded-full border',
                state === 'done' && 'border-success bg-success text-surface',
                state === 'active' && 'border-primary text-primary',
                state === 'failed' && 'border-destructive bg-destructive text-surface',
                state === 'pending' && 'text-muted-foreground')}>
                {state === 'done' ? <CheckIcon className="size-4"/>
                  : state === 'active' ? <Loader2Icon className="size-4 animate-spin"/>
                  : state === 'failed' ? <XIcon className="size-4"/>
                  : <CircleIcon className="size-2 fill-current"/>}
              </div>
              <span className={cn('text-xs', state === 'pending' ? 'text-muted-foreground' : 'font-medium')}>{s.label}</span>
            </li>
          )
        })}
      </ol>

      {status === 'failed' && error && (
        <div className="rounded-lg border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">{error}</div>
      )}

      {progress && (
        <div className="grid grid-cols-4 gap-3">
          <Stat label={phase === 'dump' ? 'Tables dumped' : 'Tables'} value={`${progress.tablesDone} / ${progress.tablesTotal}`}/>
          <Stat label={phase === 'dump' ? 'Dumped (est.)' : 'Data (est.)'}
                value={`${formatBytes(progress.bytesDone)} / ${formatBytes(progress.bytesTotal)}`}/>
          <Stat label="Throughput" value={running && transferring ? `${formatBytes(Math.round(rate))}/s` : '—'}
                hint={parallel ? `${progress.active.length} table(s) in flight` : `${formatBytes(progress.streamed)} streamed`}/>
          <Stat label="Indexes & constraints" value={`${progress.objectsDone} / ~${progress.objectsTotal}`}
                hint={progress.errors > 0 ? `${progress.errors} error(s), see raw log` : undefined}
                hintClassName={progress.errors > 0 ? 'text-destructive' : undefined}/>
        </div>
      )}

      {plan && plan.tables.length > 0 && (
        <TableList plan={plan} progress={progress} doneTables={doneTables} success={status === 'success'} running={running}/>
      )}
    </div>
  )
}

function Stat({label, value, hint, hintClassName}: {label: string; value: string; hint?: string; hintClassName?: string}) {
  return (
    <div className="rounded-lg border px-3 py-2">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="truncate text-sm font-medium tabular-nums">{value}</div>
      {hint && <div className={cn('truncate text-xs text-muted-foreground', hintClassName)}>{hint}</div>}
    </div>
  )
}

function TableList({plan, progress, doneTables, success, running}: {
  plan: CopyPlan
  progress: Progress | null
  doneTables: Set<string>
  success: boolean
  running: boolean
}) {
  const active = new Map((progress?.active ?? []).map(a => [a.name, a]))
  const dumping = progress?.phase === 'dump'
  const pastData = success || progress?.phase === 'finalize' || progress?.phase === 'done'
  const parallel = progress?.parallel ?? false
  const rows = plan.tables.slice(0, MAX_TABLE_ROWS)
  const largest = plan.tables[0]?.bytes || 1

  return (
    <div className="grid gap-1.5">
      <div className="text-xs text-muted-foreground">Tables by size{dumping && ' · dumping to disk'}</div>
      <div className="grid gap-1">
        {rows.map(t => {
          const done = !dumping && (pastData || doneTables.has(t.name))
          const a = running && !done ? active.get(t.name) : undefined
          const isActive = !!a
          const fill = done ? 1 : a && a.bytes > 0 ? a.done / a.bytes : 0
          return (
            <div key={t.name} className={cn('grid grid-cols-[1rem_minmax(0,1fr)_5rem_8rem] items-center gap-2 rounded px-1.5 py-0.5 text-xs',
              isActive && 'bg-muted')}>
              {done ? <CheckIcon className="size-3.5 text-success"/>
                : isActive ? <Loader2Icon className="size-3.5 animate-spin text-primary"/>
                : <CircleIcon className="size-1.5 justify-self-center fill-current text-muted-foreground"/>}
              <span className={cn('truncate font-mono', !done && !isActive && 'text-muted-foreground')}>{t.name}</span>
              <span className="text-right text-muted-foreground tabular-nums">{formatBytes(t.bytes)}</span>
              {/* Track width shows relative table size; fill shows how much of it is copied. */}
              <div className="h-1.5 rounded-full bg-muted" style={{width: `${Math.max(4, (t.bytes / largest) * 100)}%`}}>
                {/* Parallel workers report no partial progress, so in-flight tables pulse instead. */}
                {isActive && parallel
                  ? <div className="h-full w-full animate-pulse rounded-full bg-primary/50"/>
                  : <div className={cn('h-full rounded-full', done ? 'bg-success' : 'bg-primary')} style={{width: `${fill * 100}%`}}/>}
              </div>
            </div>
          )
        })}
      </div>
      {plan.tables.length > MAX_TABLE_ROWS && (
        <div className="text-xs text-muted-foreground">and {plan.tables.length - MAX_TABLE_ROWS} smaller table(s)</div>
      )}
    </div>
  )
}
