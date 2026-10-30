import {useEffect, useState} from 'react'
import {toast} from 'sonner'
import {CheckCircle2Icon, XCircleIcon} from 'lucide-react'
import {Button} from '@/components/ui/button'
import {Card, CardContent, CardDescription, CardHeader, CardTitle} from '@/components/ui/card'
import {Input} from '@/components/ui/input'
import {Label} from '@/components/ui/label'
import {api, errorMessage, Settings, SettingsInfo, ToolInfo} from '@/lib/api'

export function SettingsPage() {
  const [info, setInfo] = useState<SettingsInfo | null>(null)
  const [form, setForm] = useState<Settings>({pgDumpPath: '', pgRestorePath: ''})

  useEffect(() => {
    api.getSettings().then(i => { setInfo(i); setForm(i.settings) })
  }, [])

  async function save() {
    try {
      const i = await api.saveSettings(form)
      setInfo(i)
      toast.success('Settings saved')
    } catch (e) {
      toast.error('Save failed', {description: errorMessage(e)})
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <header>
        <h1 className="text-xl font-semibold">Settings</h1>
        <p className="text-sm text-muted-foreground">PostgreSQL client tools used for copying.</p>
      </header>

      <Card>
        <CardHeader>
          <CardTitle>Client binaries</CardTitle>
          <CardDescription>
            Leave empty to use the ones on PATH. pg_dump must be the same major version as the source server or newer.
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-4">
          <ToolField label="pg_dump path" value={form.pgDumpPath} tool={info?.pgDump}
                     onChange={v => setForm(f => ({...f, pgDumpPath: v}))}/>
          <ToolField label="pg_restore path" value={form.pgRestorePath} tool={info?.pgRestore}
                     onChange={v => setForm(f => ({...f, pgRestorePath: v}))}/>
          <div className="flex justify-end">
            <Button onClick={save}>Save</Button>
          </div>
        </CardContent>
      </Card>

      {info && (
        <p className="text-xs text-muted-foreground">
          Config directory: <span className="font-mono">{info.configDir}</span>
        </p>
      )}
    </div>
  )
}

function ToolField({label, value, tool, onChange}: {
  label: string
  value: string
  tool?: ToolInfo
  onChange: (v: string) => void
}) {
  return (
    <div className="grid gap-1.5">
      <Label className="text-xs text-muted-foreground">{label}</Label>
      <Input value={value} onChange={e => onChange(e.target.value)} placeholder={tool?.path || 'auto-detect'} className="font-mono"/>
      {tool && (tool.error
        ? <p className="flex items-center gap-1.5 text-xs text-destructive"><XCircleIcon className="size-3.5"/>{tool.error}</p>
        : <p className="flex items-center gap-1.5 text-xs text-muted-foreground">
            <CheckCircle2Icon className="size-3.5 text-success"/>{tool.version} <span className="font-mono">({tool.path})</span>
          </p>)}
    </div>
  )
}
