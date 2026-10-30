import {useEffect, useState} from 'react'
import {toast} from 'sonner'
import {FolderOpenIcon, Loader2Icon, PlugZapIcon} from 'lucide-react'
import {Button} from '@/components/ui/button'
import {Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle} from '@/components/ui/dialog'
import {Input} from '@/components/ui/input'
import {Label} from '@/components/ui/label'
import {Select, SelectContent, SelectItem, SelectTrigger, SelectValue} from '@/components/ui/select'
import {Switch} from '@/components/ui/switch'
import {Tabs, TabsContent, TabsList, TabsTrigger} from '@/components/ui/tabs'
import {ColorDot} from '@/components/ColorDot'
import {api, AuthMethod, Connection, connectionColors, emptyConnection, errorMessage} from '@/lib/api'

interface Props {
  open: boolean
  initial: Connection | null
  onOpenChange: (open: boolean) => void
  onSaved: (c: Connection) => void
}

export function ConnectionDialog({open, initial, onOpenChange, onSaved}: Props) {
  const [c, setC] = useState<Connection>(emptyConnection())
  const [testing, setTesting] = useState(false)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (open) setC(initial ? structuredClone(initial) : emptyConnection())
  }, [open, initial])

  const set = <K extends keyof Connection>(k: K, v: Connection[K]) => setC(prev => ({...prev, [k]: v}))
  const setSSH = <K extends keyof Connection['ssh']>(k: K, v: Connection['ssh'][K]) =>
    setC(prev => ({...prev, ssh: {...prev.ssh, [k]: v}}))

  const valid = c.name.trim() && c.host.trim() && c.database.trim() && c.user.trim() &&
    (!c.ssh.enabled || (c.ssh.host.trim() && c.ssh.user.trim()))

  async function test() {
    setTesting(true)
    try {
      const version = await api.testConnection(c)
      toast.success('Connection OK', {description: version})
    } catch (e) {
      toast.error('Connection failed', {description: errorMessage(e)})
    } finally {
      setTesting(false)
    }
  }

  async function save() {
    setSaving(true)
    try {
      const saved = await api.saveConnection({...c, name: c.name.trim()})
      onSaved(saved)
      onOpenChange(false)
    } catch (e) {
      toast.error('Save failed', {description: errorMessage(e)})
    } finally {
      setSaving(false)
    }
  }

  async function pickKey() {
    const path = await api.pickKeyFile()
    if (path) setSSH('keyPath', path)
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{initial ? 'Edit connection' : 'New connection'}</DialogTitle>
          <DialogDescription>PostgreSQL connection, optionally through an SSH tunnel.</DialogDescription>
        </DialogHeader>

        <Tabs defaultValue="general">
          <TabsList className="w-full">
            <TabsTrigger value="general">General</TabsTrigger>
            <TabsTrigger value="ssh">SSH tunnel{c.ssh.enabled && ' •'}</TabsTrigger>
          </TabsList>

          <TabsContent value="general" className="grid gap-3 pt-3">
            <div className="grid grid-cols-[1fr_auto] gap-3">
              <Field label="Name">
                <Input value={c.name} onChange={e => set('name', e.target.value)} placeholder="Production" autoFocus/>
              </Field>
              <Field label="Color">
                <Select value={c.color || 'none'} onValueChange={v => set('color', v === 'none' ? '' : v)}>
                  <SelectTrigger className="w-28"><SelectValue/></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="none"><ColorDot color=""/> None</SelectItem>
                    {Object.keys(connectionColors).map(col => (
                      <SelectItem key={col} value={col}><ColorDot color={col}/> {col}</SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
            </div>
            <div className="grid grid-cols-[1fr_6rem] gap-3">
              <Field label="Host">
                <Input value={c.host} onChange={e => set('host', e.target.value)} placeholder="localhost"/>
              </Field>
              <Field label="Port">
                <Input type="number" value={c.port || ''} onChange={e => set('port', Number(e.target.value))}/>
              </Field>
            </div>
            <Field label="Database">
              <Input value={c.database} onChange={e => set('database', e.target.value)} placeholder="app"/>
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field label="User">
                <Input value={c.user} onChange={e => set('user', e.target.value)}/>
              </Field>
              <Field label="Password">
                <Input type="password" value={c.password ?? ''} onChange={e => set('password', e.target.value)}/>
              </Field>
            </div>
            <Field label="SSL mode">
              <Select value={c.sslMode || 'prefer'} onValueChange={v => set('sslMode', v)}>
                <SelectTrigger className="w-full"><SelectValue/></SelectTrigger>
                <SelectContent>
                  {['disable', 'allow', 'prefer', 'require', 'verify-ca', 'verify-full'].map(m => (
                    <SelectItem key={m} value={m}>{m}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </Field>
          </TabsContent>

          <TabsContent value="ssh" className="grid gap-3 pt-3">
            <label className="flex items-center justify-between rounded-lg border p-3">
              <div>
                <div className="text-sm font-medium">Use SSH tunnel</div>
                <div className="text-xs text-muted-foreground">Host/port on General tab are resolved from the SSH server.</div>
              </div>
              <Switch checked={c.ssh.enabled} onCheckedChange={v => setSSH('enabled', v)}/>
            </label>
            <fieldset disabled={!c.ssh.enabled} className="grid gap-3 disabled:opacity-50">
              <div className="grid grid-cols-[1fr_6rem] gap-3">
                <Field label="SSH host">
                  <Input value={c.ssh.host} onChange={e => setSSH('host', e.target.value)} placeholder="bastion.example.com"/>
                </Field>
                <Field label="Port">
                  <Input type="number" value={c.ssh.port || ''} onChange={e => setSSH('port', Number(e.target.value))}/>
                </Field>
              </div>
              <Field label="SSH user">
                <Input value={c.ssh.user} onChange={e => setSSH('user', e.target.value)}/>
              </Field>
              <Field label="Authentication">
                <Select value={c.ssh.authMethod} onValueChange={v => setSSH('authMethod', v as AuthMethod)}>
                  <SelectTrigger className="w-full"><SelectValue/></SelectTrigger>
                  <SelectContent>
                    <SelectItem value="key">Private key</SelectItem>
                    <SelectItem value="password">Password</SelectItem>
                    <SelectItem value="agent">SSH agent</SelectItem>
                  </SelectContent>
                </Select>
              </Field>
              {c.ssh.authMethod === 'password' && (
                <Field label="SSH password">
                  <Input type="password" value={c.ssh.password ?? ''} onChange={e => setSSH('password', e.target.value)}/>
                </Field>
              )}
              {c.ssh.authMethod === 'key' && (
                <>
                  <Field label="Private key file">
                    <div className="flex gap-2">
                      <Input value={c.ssh.keyPath} onChange={e => setSSH('keyPath', e.target.value)}/>
                      <Button type="button" variant="outline" size="icon" onClick={pickKey}><FolderOpenIcon/></Button>
                    </div>
                  </Field>
                  <Field label="Passphrase (optional)">
                    <Input type="password" value={c.ssh.passphrase ?? ''} onChange={e => setSSH('passphrase', e.target.value)}/>
                  </Field>
                </>
              )}
            </fieldset>
          </TabsContent>
        </Tabs>

        <DialogFooter className="sm:justify-between">
          <Button variant="outline" onClick={test} disabled={!valid || testing}>
            {testing ? <Loader2Icon className="animate-spin"/> : <PlugZapIcon/>} Test
          </Button>
          <div className="flex gap-2">
            <Button variant="ghost" onClick={() => onOpenChange(false)}>Cancel</Button>
            <Button onClick={save} disabled={!valid || saving}>Save</Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function Field({label, children}: {label: string; children: React.ReactNode}) {
  return (
    <div className="grid gap-1.5">
      <Label className="text-xs text-muted-foreground">{label}</Label>
      {children}
    </div>
  )
}
