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

export function describeConnection(c: Connection) {
  const db = `${c.user}@${c.host}:${c.port}/${c.database}`
  return c.ssh.enabled ? `${db} via ${c.ssh.user}@${c.ssh.host}` : db
}
