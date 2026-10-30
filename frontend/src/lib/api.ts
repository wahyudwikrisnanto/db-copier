import * as App from '../../wailsjs/go/main/App'
import {EventsOn} from '../../wailsjs/runtime/runtime'

export type AuthMethod = 'password' | 'key' | 'agent'

export interface SSHConfig {
  enabled: boolean
  host: string
  port: number
  user: string
  authMethod: AuthMethod
  keyPath: string
  password?: string
  passphrase?: string
}

export interface Connection {
  id: string
  name: string
  color: string
  host: string
  port: number
  database: string
  user: string
  password?: string
  sslMode: string
  ssh: SSHConfig
}

export interface CopyOptions {
  noOwner: boolean
  noAcl: boolean
  exitOnError: boolean
  // > 1: dump to a temporary directory and restore with this many parallel jobs.
  jobs: number
}

export interface ToolInfo {
  path: string
  version: string
  error: string
}

export interface Settings {
  pgDumpPath: string
  pgRestorePath: string
}

export interface SettingsInfo {
  settings: Settings
  pgDump: ToolInfo
  pgRestore: ToolInfo
  configDir: string
}

export interface CopyResult {
  ok: boolean
  error: string
  durationMs: number
  jobId: string
}

export type Phase = 'connect' | 'dump' | 'erase' | 'schema' | 'data' | 'finalize' | 'done'

export interface CopyTable {
  name: string
  bytes: number
}

export interface CopyPlan {
  tables: CopyTable[]
  postData: number
}

// A table being dumped or restored right now; done is only estimated when streaming.
export interface ActiveTable {
  name: string
  bytes: number
  done: number
}

// Snapshot of a running copy; data byte counts are estimates. Table and byte
// counters cover the current stage (the dump in 'dump', the restore afterwards).
export interface CopyProgress {
  phase: Phase
  parallel: boolean
  tablesDone: number
  tablesTotal: number
  bytesDone: number
  bytesTotal: number
  streamed: number
  active: ActiveTable[]
  schemaObjects: number
  objectsDone: number
  objectsTotal: number
  errors: number
}

// 'query' is reserved for the future query executor.
export type JobKind = 'copy' | 'query'
export type JobStatus = 'running' | 'completed' | 'failed' | 'cancelled'

// Snapshot of a connection taken when the job ran; never holds secrets.
export interface ConnRef {
  id: string
  name: string
  color: string
  host: string
  port: number
  database: string
  user: string
  sshHost?: string
}

export interface CopyInfo {
  options: CopyOptions
  tablesTotal: number
  tablesDone: number
  bytesTotal: number
  errors: number
}

export interface Job {
  id: string
  kind: JobKind
  status: JobStatus
  startedAt: string
  finishedAt?: string
  durationMs: number
  error?: string
  source?: ConnRef
  target?: ConnRef
  copy?: CopyInfo
}

export interface CopyDetail {
  plan?: CopyPlan
  progress?: CopyProgress
  doneTables: string[]
}

export interface JobDetail extends Job {
  copyDetail?: CopyDetail
  log: string[]
  logTruncated: boolean
  logPath: string
}

export const emptyConnection = (): Connection => ({
  id: '',
  name: '',
  color: '',
  host: 'localhost',
  port: 5432,
  database: '',
  user: 'postgres',
  password: '',
  sslMode: 'prefer',
  ssh: {enabled: false, host: '', port: 22, user: '', authMethod: 'key', keyPath: '~/.ssh/id_ed25519', password: '', passphrase: ''},
})

// The generated bindings use classes; the backend only needs the JSON shape.
export const api = {
  listConnections: () => App.ListConnections() as unknown as Promise<Connection[]>,
  saveConnection: (c: Connection) => App.SaveConnection(c as any) as unknown as Promise<Connection>,
  deleteConnection: (id: string) => App.DeleteConnection(id),
  testConnection: (c: Connection) => App.TestConnection(c as any),
  pickKeyFile: () => App.PickKeyFile(),
  getSettings: () => App.GetSettings() as unknown as Promise<SettingsInfo>,
  saveSettings: (s: Settings) => App.SaveSettings(s as any) as unknown as Promise<SettingsInfo>,
  startCopy: (sourceId: string, targetId: string, opts: CopyOptions) => App.StartCopy(sourceId, targetId, opts as any),
  cancelCopy: () => App.CancelCopy(),
  isCopyRunning: () => App.IsCopyRunning(),
  onCopyLog: (fn: (lines: string[]) => void) => EventsOn('copy:log', fn),
  onCopyDone: (fn: (res: CopyResult) => void) => EventsOn('copy:done', fn),
  onCopyPlan: (fn: (plan: CopyPlan) => void) => EventsOn('copy:plan', fn),
  onCopyProgress: (fn: (p: CopyProgress) => void) => EventsOn('copy:progress', fn),
  onCopyTables: (fn: (names: string[]) => void) => EventsOn('copy:tables', fn),
  listJobs: () => App.ListJobs() as unknown as Promise<Job[]>,
  getJob: (id: string) => App.GetJob(id) as unknown as Promise<JobDetail>,
  deleteJob: (id: string) => App.DeleteJob(id),
  clearHistory: () => App.ClearHistory(),
  exportJobLog: (id: string) => App.ExportJobLog(id),
  onHistoryChanged: (fn: () => void) => EventsOn('history:changed', fn),
}

export const errorMessage = (e: unknown) => (e instanceof Error ? e.message : String(e))

export const connectionColors: Record<string, string> = {
  red: 'bg-red-500',
  orange: 'bg-orange-500',
  yellow: 'bg-yellow-500',
  green: 'bg-green-500',
  blue: 'bg-blue-500',
  violet: 'bg-violet-500',
}

export function describeConnRef(c: ConnRef) {
  const db = `${c.user}@${c.host}:${c.port}/${c.database}`
  return c.sshHost ? `${db} via ${c.sshHost}` : db
}

export function describeConnection(c: Connection) {
  const db = `${c.user}@${c.host}:${c.port}/${c.database}`
  return c.ssh.enabled ? `${db} via ${c.ssh.user}@${c.ssh.host}` : db
}
