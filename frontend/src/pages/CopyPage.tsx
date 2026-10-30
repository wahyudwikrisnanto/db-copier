import {useEffect, useState} from 'react'
import {toast} from 'sonner'
import {ArrowRightIcon, CopyIcon, Loader2Icon, SquareIcon, TriangleAlertIcon} from 'lucide-react'
import {Button} from '@/components/ui/button'
import {Badge} from '@/components/ui/badge'
import {Card, CardContent, CardHeader} from '@/components/ui/card'
import {Tabs, TabsContent, TabsList, TabsTrigger} from '@/components/ui/tabs'
import {Input} from '@/components/ui/input'
import {Label} from '@/components/ui/label'
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '@/components/ui/select'
import {Switch} from '@/components/ui/switch'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
  AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {ColorDot} from '@/components/ColorDot'
import {CopyProgress, RunStatus as Status} from '@/components/CopyProgress'
import {LogView} from '@/components/LogView'
import {cn} from '@/lib/utils'
import {api, Connection, CopyOptions, CopyPlan, CopyProgress as Progress, describeConnection, errorMessage} from '@/lib/api'

type View = 'progress' | 'log'

const MAX_LOG_LINES = 5000
const PREFS_KEY = 'copy-prefs'
const VIEW_KEY = 'copy-view'

interface Prefs {
  sourceId: string
  targetId: string
  options: CopyOptions
}

function loadPrefs(): Prefs {
  const fallback: Prefs = {sourceId: '', targetId: '', options: {noOwner: true, noAcl: true, exitOnError: true, jobs: 4}}
  try {
    const raw = localStorage.getItem(PREFS_KEY)
    if (!raw) return fallback
    const saved = JSON.parse(raw)
    return {...fallback, ...saved, options: {...fallback.options, ...saved.options}}
  } catch {
    return fallback
  }
}

export function CopyPage({connections, onOpenJob}: {connections: Connection[]; onOpenJob: (id: string) => void}) {
  const [prefs, setPrefs] = useState<Prefs>(loadPrefs)
  const [status, setStatus] = useState<Status>('idle')
  const [log, setLog] = useState<string[]>([])
  const [view, setView] = useState<View>(() => (localStorage.getItem(VIEW_KEY) === 'log' ? 'log' : 'progress'))
  const [plan, setPlan] = useState<CopyPlan | null>(null)
  const [progress, setProgress] = useState<Progress | null>(null)
  const [doneTables, setDoneTables] = useState<Set<string>>(() => new Set())
  const [startedAt, setStartedAt] = useState<number | null>(null)
  const [finishedAt, setFinishedAt] = useState<number | null>(null)
  const [error, setError] = useState('')
  const [confirmOpen, setConfirmOpen] = useState(false)
  const [confirmText, setConfirmText] = useState('')

  const source = connections.find(c => c.id === prefs.sourceId)
  const target = connections.find(c => c.id === prefs.targetId)
  const running = status === 'running'

  useEffect(() => {
    try { localStorage.setItem(PREFS_KEY, JSON.stringify(prefs)) } catch { /* ignore */ }
  }, [prefs])

  useEffect(() => {
    try { localStorage.setItem(VIEW_KEY, view) } catch { /* ignore */ }
  }, [view])

  useEffect(() => {
    api.isCopyRunning().then(r => {
      if (!r) return
      setStatus('running')
      setStartedAt(t => t ?? Date.now())
    })
    const offPlan = api.onCopyPlan(setPlan)
    const offProgress = api.onCopyProgress(setProgress)
    const offTables = api.onCopyTables(names => setDoneTables(prev => {
      const next = new Set(prev)
      names.forEach(n => next.add(n))
      return next
    }))
    const offLog = api.onCopyLog(lines => setLog(prev => {
      const next = prev.concat(lines)
      return next.length > MAX_LOG_LINES ? next.slice(-MAX_LOG_LINES) : next
    }))
    const offDone = api.onCopyDone(res => {
      setStatus(res.ok ? 'success' : 'failed')
      setFinishedAt(Date.now())
      setError(res.ok ? '' : res.error)
      const secs = (res.durationMs / 1000).toFixed(1)
      const action = res.jobId ? {label: 'Details', onClick: () => onOpenJob(res.jobId)} : undefined
      if (res.ok) toast.success('Copy finished', {description: `Completed in ${secs}s`, action})
      else toast.error('Copy failed', {description: res.error, action})
    })
    return () => { offLog(); offDone(); offPlan(); offProgress(); offTables() }
  }, [])

  const setOption = (k: keyof CopyOptions, v: boolean) => setPrefs(p => ({...p, options: {...p.options, [k]: v}}))
  const sameConnection = !!source && !!target && source.id === target.id
  const canStart = !!source && !!target && !sameConnection && !running

  async function start() {
    if (!source || !target) return
    setConfirmOpen(false)
    setLog([])
    setPlan(null)
    setProgress(null)
    setDoneTables(new Set())
    setStartedAt(Date.now())
    setFinishedAt(null)
    setError('')
    setStatus('running')
    try {
      await api.startCopy(source.id, target.id, prefs.options)
    } catch (e) {
      setStatus('failed')
      setFinishedAt(Date.now())
      setError(errorMessage(e))
      toast.error('Could not start copy', {description: errorMessage(e)})
    }
  }

  return (
    <div className="flex h-full flex-col gap-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Copy database</h1>
          <p className="text-sm text-muted-foreground">Erase the target, then copy everything from the source into it.</p>
        </div>
        <StatusBadge status={status}/>
      </header>

      <Card>
        <CardContent className="grid gap-5">
          <div className="grid grid-cols-[1fr_auto_1fr] items-end gap-3">
            <ConnectionPicker label="Source (server)" labelClassName="text-source" value={prefs.sourceId} connections={connections} disabled={running}
                              onChange={id => setPrefs(p => ({...p, sourceId: id}))}/>
            <ArrowRightIcon className="mb-2 size-4 text-muted-foreground"/>
            <ConnectionPicker label="Target (local, will be erased)" labelClassName="text-target" value={prefs.targetId} connections={connections} disabled={running}
                              onChange={id => setPrefs(p => ({...p, targetId: id}))}/>
          </div>
          {sameConnection && <p className="text-sm text-destructive">Source and target must be different.</p>}

          <div className="grid grid-cols-4 gap-3">
            <OptionSwitch label="Skip ownership" hint="--no-owner" checked={prefs.options.noOwner} disabled={running}
                          onChange={v => setOption('noOwner', v)}/>
            <OptionSwitch label="Skip privileges" hint="--no-acl" checked={prefs.options.noAcl} disabled={running}
                          onChange={v => setOption('noAcl', v)}/>
            <OptionSwitch label="Stop on first error" hint="--exit-on-error" checked={prefs.options.exitOnError} disabled={running}
                          onChange={v => setOption('exitOnError', v)}/>
            <JobsSelect value={prefs.options.jobs} disabled={running}
                        onChange={jobs => setPrefs(p => ({...p, options: {...p.options, jobs}}))}/>
          </div>

          <div className="flex justify-end gap-2">
            {running ? (
              <Button variant="destructive" onClick={() => api.cancelCopy()}><SquareIcon/> Cancel</Button>
            ) : (
              <Button size="lg" disabled={!canStart} onClick={() => { setConfirmText(''); setConfirmOpen(true) }}>
                <CopyIcon/> Copy
              </Button>
            )}
          </div>
        </CardContent>
      </Card>

      <Card className="min-h-0 flex-1">
        <Tabs value={view} onValueChange={v => setView(v as View)} className="min-h-0 flex-1">
          <CardHeader>
            <TabsList>
              <TabsTrigger value="progress">Progress</TabsTrigger>
              <TabsTrigger value="log">Raw log</TabsTrigger>
            </TabsList>
          </CardHeader>
          <CardContent className="min-h-0 flex-1">
            <TabsContent value="progress" className="h-full overflow-auto">
              <CopyProgress status={status} plan={plan} progress={progress} doneTables={doneTables}
                            startedAt={startedAt} finishedAt={finishedAt} error={error}/>
            </TabsContent>
            <TabsContent value="log" className="h-full">
              <LogView lines={log} follow={view === 'log'} className="h-full min-h-48"/>
            </TabsContent>
          </CardContent>
        </Tabs>
      </Card>

      <AlertDialog open={confirmOpen} onOpenChange={setConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle className="flex items-center gap-2">
              <TriangleAlertIcon className="size-5 text-destructive"/> Erase target database?
            </AlertDialogTitle>
            <AlertDialogDescription asChild>
              <div className="grid gap-2">
                <p>
                  Every schema, table and extension in <b className="text-foreground">{target?.database}</b> on{' '}
                  <b className="text-foreground">{target?.name}</b> will be permanently dropped, then replaced with a copy of{' '}
                  <b className="text-foreground">{source?.name}</b>.
                </p>
                <p className="font-mono text-xs">{target && describeConnection(target)}</p>
              </div>
            </AlertDialogDescription>
          </AlertDialogHeader>
          <div className="grid gap-1.5">
            <Label className="text-xs text-muted-foreground">Type <b className="text-foreground">{target?.database}</b> to confirm</Label>
            <Input value={confirmText} onChange={e => setConfirmText(e.target.value)} autoFocus
                   onKeyDown={e => e.key === 'Enter' && confirmText === target?.database && start()}/>
          </div>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" disabled={confirmText !== target?.database} onClick={start}>
              Erase & copy
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}

function ConnectionPicker({label, labelClassName, value, connections, disabled, onChange}: {
  label: string
  labelClassName?: string
  value: string
  connections: Connection[]
  disabled: boolean
  onChange: (id: string) => void
}) {
  const known = connections.some(c => c.id === value)
  return (
    <div className="grid min-w-0 gap-1.5">
      <Label className={cn('text-xs text-muted-foreground', labelClassName)}>{label}</Label>
      <Select value={known ? value : ''} onValueChange={onChange} disabled={disabled}>
        <SelectTrigger className="w-full"><SelectValue placeholder="Select connection"/></SelectTrigger>
        <SelectContent>
          {connections.map(c => (
            <SelectItem key={c.id} value={c.id}>
              <ColorDot color={c.color}/> {c.name}
              <span className="text-xs text-muted-foreground">{c.database}</span>
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

function OptionSwitch({label, hint, checked, disabled, onChange}: {
  label: string
  hint: string
  checked: boolean
  disabled: boolean
  onChange: (v: boolean) => void
}) {
  return (
    <label className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2">
      <div>
        <div className="text-sm">{label}</div>
        <div className="font-mono text-xs text-muted-foreground">{hint}</div>
      </div>
      <Switch checked={checked} onCheckedChange={onChange} disabled={disabled}/>
    </label>
  )
}

const jobChoices = [1, 2, 4, 8]

function JobsSelect({value, disabled, onChange}: {value: number; disabled: boolean; onChange: (v: number) => void}) {
  return (
    <div className="flex items-center justify-between gap-3 rounded-lg border px-3 py-2">
      <div className="min-w-0">
        <div className="text-sm">Parallel jobs</div>
        <div className="truncate text-xs text-muted-foreground">
          {value > 1 ? 'dump to disk, then restore' : 'stream, no temp files'}
        </div>
      </div>
      <Select value={String(value)} onValueChange={v => onChange(Number(v))} disabled={disabled}>
        <SelectTrigger size="sm" className="w-16"><SelectValue/></SelectTrigger>
        <SelectContent>
          {jobChoices.map(n => <SelectItem key={n} value={String(n)}>{n}</SelectItem>)}
        </SelectContent>
      </Select>
    </div>
  )
}

function StatusBadge({status}: {status: Status}) {
  switch (status) {
    case 'running':
      return <Badge className="bg-status-running text-primary-foreground"><Loader2Icon className="animate-spin"/> Running</Badge>
    case 'success':
      return <Badge className="border-success-border bg-success-bg text-success">Success</Badge>
    case 'failed':
      return <Badge variant="destructive">Failed</Badge>
    default:
      return <Badge variant="outline">Idle</Badge>
  }
}
