import {useCallback, useEffect, useMemo, useState} from 'react'
import {toast} from 'sonner'
import {ArrowRightIcon, CopyIcon, HistoryIcon, SearchIcon, TerminalIcon, Trash2Icon} from 'lucide-react'
import {Button} from '@/components/ui/button'
import {Input} from '@/components/ui/input'
import {Tabs, TabsList, TabsTrigger} from '@/components/ui/tabs'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
  AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {ColorDot} from '@/components/ColorDot'
import {JobStatusIcon, jobStatusLabel} from '@/components/JobStatus'
import {formatBytes, formatDuration} from '@/components/CopyProgress'
import {JobDetailPage} from '@/pages/JobDetailPage'
import {cn} from '@/lib/utils'
import {formatDay, formatTime} from '@/lib/format'
import {api, ConnRef, errorMessage, Job, JobStatus} from '@/lib/api'

type Filter = 'all' | JobStatus

const filters: Filter[] = ['all', 'running', 'completed', 'failed', 'cancelled']

export function HistoryPage({jobId, onOpenJob, onOpenCopy}: {
  jobId: string | null
  onOpenJob: (id: string | null) => void
  onOpenCopy: () => void
}) {
  if (jobId) return <JobDetailPage id={jobId} onBack={() => onOpenJob(null)} onOpenCopy={onOpenCopy}/>
  return <JobList onOpenJob={onOpenJob}/>
}

function JobList({onOpenJob}: {onOpenJob: (id: string) => void}) {
  const [jobs, setJobs] = useState<Job[] | null>(null)
  const [filter, setFilter] = useState<Filter>('all')
  const [query, setQuery] = useState('')
  const [clearOpen, setClearOpen] = useState(false)

  const reload = useCallback(() => {
    api.listJobs()
      .then(js => setJobs(js ?? []))
      .catch(e => toast.error('Could not load history', {description: errorMessage(e)}))
  }, [])
  useEffect(() => {
    reload()
    return api.onHistoryChanged(reload)
  }, [reload])

  const counts = useMemo(() => {
    const c: Record<Filter, number> = {all: 0, running: 0, completed: 0, failed: 0, cancelled: 0}
    for (const j of jobs ?? []) {
      c.all++
      c[j.status]++
    }
    return c
  }, [jobs])

  const visible = useMemo(() => {
    const q = query.trim().toLowerCase()
    return (jobs ?? []).filter(j => (filter === 'all' || j.status === filter) && (!q || matches(j, q)))
  }, [jobs, filter, query])

  // Jobs arrive newest first, so consecutive runs of the same day form a group.
  const groups = useMemo(() => {
    const out: {day: string; jobs: Job[]}[] = []
    for (const j of visible) {
      const day = formatDay(j.startedAt)
      if (out.at(-1)?.day === day) out.at(-1)!.jobs.push(j)
      else out.push({day, jobs: [j]})
    }
    return out
  }, [visible])

  async function clear() {
    setClearOpen(false)
    try {
      await api.clearHistory()
    } catch (e) {
      toast.error('Could not clear history', {description: errorMessage(e)})
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Activity</h1>
          <p className="text-sm text-muted-foreground">Every copy you run, with its progress and full log.</p>
        </div>
        <Button variant="outline" disabled={counts.all === counts.running} onClick={() => setClearOpen(true)}>
          <Trash2Icon/> Clear history
        </Button>
      </header>

      <div className="flex items-center justify-between gap-3">
        <Tabs value={filter} onValueChange={v => setFilter(v as Filter)}>
          <TabsList>
            {filters.map(f => (
              <TabsTrigger key={f} value={f}>
                {f === 'all' ? 'All' : jobStatusLabel(f)}
                <span className="text-xs text-muted-foreground tabular-nums">{counts[f]}</span>
              </TabsTrigger>
            ))}
          </TabsList>
        </Tabs>
        <div className="relative w-64">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"/>
          <Input value={query} onChange={e => setQuery(e.target.value)} placeholder="Search connection or database" className="pl-8"/>
        </div>
      </div>

      {jobs === null ? null : visible.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed py-16 text-center">
          <HistoryIcon className="size-8 text-muted-foreground"/>
          <div className="text-sm text-muted-foreground">
            {jobs.length === 0 ? 'No activity yet. Copies you run will show up here.' : 'No jobs match this filter.'}
          </div>
        </div>
      ) : (
        <div className="grid gap-5">
          {groups.map(g => (
            <section key={g.day} className="grid gap-2">
              <h2 className="text-xs font-medium text-muted-foreground">{g.day}</h2>
              <div className="divide-y overflow-hidden rounded-xl border bg-card">
                {g.jobs.map(j => <JobRow key={j.id} job={j} onOpen={() => onOpenJob(j.id)}/>)}
              </div>
            </section>
          ))}
        </div>
      )}

      <AlertDialog open={clearOpen} onOpenChange={setClearOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Clear activity history?</AlertDialogTitle>
            <AlertDialogDescription>
              Deletes the record and log of every finished job. Databases are not touched. A running job is kept.
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={clear}>Clear history</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function matches(j: Job, q: string): boolean {
  return [j.source, j.target].some(c => c && [c.name, c.database, c.host].some(s => s.toLowerCase().includes(q)))
}

function JobRow({job, onOpen}: {job: Job; onOpen: () => void}) {
  const KindIcon = job.kind === 'query' ? TerminalIcon : CopyIcon
  return (
    <button onClick={onOpen}
            className="grid w-full grid-cols-[1.25rem_minmax(0,1fr)_auto] items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-accent">
      <JobStatusIcon status={job.status} className="size-5"/>
      <div className="grid min-w-0 gap-0.5">
        <div className="flex min-w-0 items-center gap-2 text-sm font-medium">
          <KindIcon className="size-3.5 shrink-0 text-muted-foreground"/>
          {job.source && <ConnName conn={job.source}/>}
          {job.source && job.target && <ArrowRightIcon className="size-3.5 shrink-0 text-muted-foreground"/>}
          {job.target && <ConnName conn={job.target}/>}
        </div>
        {job.error ? (
          <div className={cn('truncate text-xs', job.status === 'cancelled' ? 'text-muted-foreground' : 'text-danger')}>{job.error}</div>
        ) : (
          <div className="truncate text-xs text-muted-foreground">{summary(job)}</div>
        )}
      </div>
      <div className="text-right text-xs text-muted-foreground tabular-nums">
        <div>{formatTime(job.startedAt)}</div>
        <div>{job.status === 'running' ? 'in progress' : formatDuration(job.durationMs)}</div>
      </div>
    </button>
  )
}

function ConnName({conn}: {conn: ConnRef}) {
  return (
    <span className="flex min-w-0 items-center gap-1.5">
      <ColorDot color={conn.color}/>
      <span className="truncate">{conn.name}</span>
      <span className="truncate font-mono text-xs font-normal text-muted-foreground">{conn.database}</span>
    </span>
  )
}

function summary(job: Job): string {
  const c = job.copy
  if (!c || c.tablesTotal === 0) return job.status === 'running' ? 'Copying…' : 'Copy'
  const parts = [`${c.tablesDone} / ${c.tablesTotal} tables`, formatBytes(c.bytesTotal)]
  if (c.errors > 0) parts.push(`${c.errors} error(s)`)
  return parts.join(' · ')
}
