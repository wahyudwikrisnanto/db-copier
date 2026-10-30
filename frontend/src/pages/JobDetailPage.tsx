import {useCallback, useEffect, useMemo, useState} from 'react'
import {toast} from 'sonner'
import {ArrowLeftIcon, DownloadIcon, Loader2Icon, SearchIcon, Trash2Icon} from 'lucide-react'
import {Button} from '@/components/ui/button'
import {Card, CardContent, CardHeader} from '@/components/ui/card'
import {Input} from '@/components/ui/input'
import {Label} from '@/components/ui/label'
import {Switch} from '@/components/ui/switch'
import {Tabs, TabsContent, TabsList, TabsTrigger} from '@/components/ui/tabs'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
  AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {ColorDot} from '@/components/ColorDot'
import {CopyProgress, formatDuration, RunStatus} from '@/components/CopyProgress'
import {JobStatusBadge} from '@/components/JobStatus'
import {isErrorLine, LogView} from '@/components/LogView'
import {cn} from '@/lib/utils'
import {formatDateTime} from '@/lib/format'
import {api, ConnRef, CopyOptions, describeConnRef, errorMessage, JobDetail, JobStatus} from '@/lib/api'

type View = 'progress' | 'log'

const runStatus: Record<JobStatus, RunStatus> = {
  running: 'running',
  completed: 'success',
  failed: 'failed',
  cancelled: 'failed',
}

export function JobDetailPage({id, onBack, onOpenCopy}: {id: string; onBack: () => void; onOpenCopy: () => void}) {
  const [job, setJob] = useState<JobDetail | null>(null)
  const [loadError, setLoadError] = useState('')
  const [view, setView] = useState<View>('progress')
  const [deleteOpen, setDeleteOpen] = useState(false)

  const reload = useCallback(() => {
    api.getJob(id)
      .then(j => { setJob(j); setLoadError('') })
      .catch(e => setLoadError(errorMessage(e)))
  }, [id])
  useEffect(() => {
    setJob(null)
    reload()
    return api.onHistoryChanged(reload)
  }, [reload])

  async function exportLog() {
    try {
      const path = await api.exportJobLog(id)
      if (path) toast.success('Log exported', {description: path})
    } catch (e) {
      toast.error('Export failed', {description: errorMessage(e)})
    }
  }

  async function remove() {
    setDeleteOpen(false)
    try {
      await api.deleteJob(id)
      onBack()
    } catch (e) {
      toast.error('Delete failed', {description: errorMessage(e)})
    }
  }

  const back = <Button variant="ghost" size="sm" className="-ml-2 w-fit text-muted-foreground" onClick={onBack}><ArrowLeftIcon/> Activity</Button>

  if (!job) {
    return (
      <div className="flex flex-col gap-4">
        {back}
        <div className="text-sm text-muted-foreground">{loadError || 'Loading…'}</div>
      </div>
    )
  }

  const running = job.status === 'running'
  const detail = job.copyDetail
  const options = job.copy?.options

  return (
    <div className="flex h-full flex-col gap-6">
      <div className="grid gap-2">
        {back}
        <header className="flex items-start justify-between gap-4">
          <div className="min-w-0">
            <div className="flex items-center gap-3">
              <h1 className="truncate text-xl font-semibold">
                {job.kind === 'query' ? 'Query' : 'Copy'} {job.source?.name ?? ''}{job.source && job.target && ' → '}{job.target?.name ?? ''}
              </h1>
              <JobStatusBadge status={job.status}/>
            </div>
            <p className="font-mono text-xs text-muted-foreground">{job.id}</p>
          </div>
          <div className="flex shrink-0 gap-2">
            <Button variant="outline" onClick={exportLog}><DownloadIcon/> Export log</Button>
            <Button variant="outline" disabled={running} onClick={() => setDeleteOpen(true)}><Trash2Icon/> Delete</Button>
          </div>
        </header>
      </div>

      {running && (
        <div className="flex items-center justify-between gap-3 rounded-lg border border-info-border bg-info-bg px-3 py-2 text-sm text-info">
          <span className="flex items-center gap-2"><Loader2Icon className="size-4 animate-spin"/> This copy is still running.</span>
          <Button size="sm" variant="outline" onClick={onOpenCopy}>Watch live</Button>
        </div>
      )}

      <div className="grid grid-cols-2 gap-3">
        {job.source && <ConnCard label="Source" conn={job.source} className="border-l-source" labelClassName="text-source"/>}
        {job.target && <ConnCard label="Target (erased)" conn={job.target} className="border-l-target" labelClassName="text-target"/>}
      </div>

      <div className="grid grid-cols-4 gap-3">
        <Meta label="Started" value={formatDateTime(job.startedAt)}/>
        <Meta label="Finished" value={job.finishedAt ? formatDateTime(job.finishedAt) : '—'}/>
        <Meta label="Duration" value={running ? '—' : formatDuration(job.durationMs)}/>
        <Meta label="Options" value={options ? optionFlags(options).join(' ') || 'none' : '—'} mono/>
      </div>

      <Card className="min-h-0 flex-1">
        <Tabs value={detail ? view : 'log'} onValueChange={v => setView(v as View)} className="min-h-0 flex-1">
          <CardHeader>
            <TabsList>
              {detail && <TabsTrigger value="progress">Progress</TabsTrigger>}
              <TabsTrigger value="log">Log</TabsTrigger>
            </TabsList>
          </CardHeader>
          <CardContent className="min-h-0 flex-1">
            {detail && (
              <TabsContent value="progress" className="h-full overflow-auto">
                <CopyProgress status={runStatus[job.status]} plan={detail.plan ?? null} progress={detail.progress ?? null}
                              doneTables={new Set(detail.doneTables)} startedAt={Date.parse(job.startedAt)}
                              finishedAt={job.finishedAt ? Date.parse(job.finishedAt) : null} error={job.error ?? ''}/>
              </TabsContent>
            )}
            <TabsContent value="log" className="h-full">
              <JobLog job={job}/>
            </TabsContent>
          </CardContent>
        </Tabs>
      </Card>

      <AlertDialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete this job?</AlertDialogTitle>
            <AlertDialogDescription>Removes the record and its log. The databases are not touched.</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={remove}>Delete</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function JobLog({job}: {job: JobDetail}) {
  const [query, setQuery] = useState('')
  const [errorsOnly, setErrorsOnly] = useState(false)
  const errorCount = useMemo(() => job.log.filter(isErrorLine).length, [job.log])
  const lines = useMemo(() => {
    const q = query.trim().toLowerCase()
    return job.log.filter(l => (!errorsOnly || isErrorLine(l)) && (!q || l.toLowerCase().includes(q)))
  }, [job.log, query, errorsOnly])

  return (
    <div className="flex h-full min-h-64 flex-col gap-3">
      <div className="flex items-center gap-4">
        <div className="relative w-72">
          <SearchIcon className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"/>
          <Input value={query} onChange={e => setQuery(e.target.value)} placeholder="Filter log" className="pl-8"/>
        </div>
        <Label className="flex items-center gap-2 text-sm font-normal">
          <Switch checked={errorsOnly} onCheckedChange={setErrorsOnly}/>
          Errors only <span className={cn('tabular-nums', errorCount > 0 ? 'text-danger' : 'text-muted-foreground')}>({errorCount})</span>
        </Label>
        <span className="ml-auto text-xs text-muted-foreground tabular-nums">
          {lines.length === job.log.length ? `${job.log.length} lines` : `${lines.length} of ${job.log.length} lines`}
        </span>
      </div>
      {job.logTruncated && (
        <div className="rounded-lg border border-warning-border bg-warning-bg px-3 py-2 text-xs text-warning">
          Showing the last {job.log.length} lines. Export the log for the full output.
        </div>
      )}
      <LogView lines={lines} className="min-h-0 flex-1"/>
    </div>
  )
}

function ConnCard({label, conn, className, labelClassName}: {label: string; conn: ConnRef; className?: string; labelClassName?: string}) {
  return (
    <div className={cn('grid gap-1 rounded-lg border border-l-4 bg-card px-3 py-2', className)}>
      <div className={cn('text-xs font-medium', labelClassName)}>{label}</div>
      <div className="flex items-center gap-2 text-sm font-medium"><ColorDot color={conn.color}/> {conn.name}</div>
      <div className="truncate font-mono text-xs text-muted-foreground">{describeConnRef(conn)}</div>
    </div>
  )
}

function Meta({label, value, mono}: {label: string; value: string; mono?: boolean}) {
  return (
    <div className="rounded-lg border bg-card px-3 py-2">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className={cn('truncate text-sm font-medium tabular-nums', mono && 'font-mono text-xs')}>{value}</div>
    </div>
  )
}

function optionFlags(o: CopyOptions): string[] {
  return [o.noOwner && '--no-owner', o.noAcl && '--no-acl', o.exitOnError && '--exit-on-error'].filter(Boolean) as string[]
}
