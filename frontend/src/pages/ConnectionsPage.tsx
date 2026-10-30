import {useState} from 'react'
import {toast} from 'sonner'
import {DatabaseIcon, PencilIcon, PlusIcon, Trash2Icon} from 'lucide-react'
import {Button} from '@/components/ui/button'
import {Badge} from '@/components/ui/badge'
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription,
  AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import {ColorDot} from '@/components/ColorDot'
import {ConnectionDialog} from '@/components/ConnectionDialog'
import {api, Connection, describeConnection, errorMessage} from '@/lib/api'

export function ConnectionsPage({connections, reload}: {connections: Connection[]; reload: () => void}) {
  const [editing, setEditing] = useState<Connection | null>(null)
  const [dialogOpen, setDialogOpen] = useState(false)
  const [deleting, setDeleting] = useState<Connection | null>(null)

  function openNew() {
    setEditing(null)
    setDialogOpen(true)
  }

  function openEdit(c: Connection) {
    setEditing(c)
    setDialogOpen(true)
  }

  async function confirmDelete() {
    if (!deleting) return
    try {
      await api.deleteConnection(deleting.id)
      reload()
    } catch (e) {
      toast.error('Delete failed', {description: errorMessage(e)})
    }
    setDeleting(null)
  }

  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold">Connections</h1>
          <p className="text-sm text-muted-foreground">Saved PostgreSQL servers. Passwords are kept in the OS keyring.</p>
        </div>
        <Button onClick={openNew}><PlusIcon/> New connection</Button>
      </header>

      {connections.length === 0 ? (
        <div className="flex flex-col items-center gap-3 rounded-xl border border-dashed py-16 text-center">
          <DatabaseIcon className="size-8 text-muted-foreground"/>
          <div className="text-sm text-muted-foreground">No connections yet.</div>
          <Button variant="outline" onClick={openNew}><PlusIcon/> Add your first connection</Button>
        </div>
      ) : (
        <div className="divide-y rounded-xl border">
          {connections.map(c => (
            <div key={c.id} className="group flex items-center gap-3 px-4 py-3 hover:bg-muted/40">
              <ColorDot color={c.color}/>
              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2 text-sm font-medium">
                  {c.name}
                  {c.ssh.enabled && <Badge variant="secondary">SSH</Badge>}
                </div>
                <div className="truncate font-mono text-xs text-muted-foreground">{describeConnection(c)}</div>
              </div>
              <Button variant="ghost" size="icon-sm" onClick={() => openEdit(c)}><PencilIcon/></Button>
              <Button variant="ghost" size="icon-sm" onClick={() => setDeleting(c)}><Trash2Icon/></Button>
            </div>
          ))}
        </div>
      )}

      <ConnectionDialog open={dialogOpen} initial={editing} onOpenChange={setDialogOpen} onSaved={reload}/>

      <AlertDialog open={!!deleting} onOpenChange={o => !o && setDeleting(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>Delete “{deleting?.name}”?</AlertDialogTitle>
            <AlertDialogDescription>Removes the saved connection only. The database is not touched.</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>Cancel</AlertDialogCancel>
            <AlertDialogAction variant="destructive" onClick={confirmDelete}>Delete</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  )
}
