import {useCallback, useEffect, useState} from 'react'
import {ThemeProvider, useTheme} from 'next-themes'
import {CopyIcon, DatabaseIcon, HistoryIcon, MoonIcon, SettingsIcon, SunIcon} from 'lucide-react'
import {Toaster} from '@/components/ui/sonner'
import {TooltipProvider} from '@/components/ui/tooltip'
import {Button} from '@/components/ui/button'
import {cn} from '@/lib/utils'
import {api, Connection} from '@/lib/api'
import {CopyPage} from '@/pages/CopyPage'
import {ConnectionsPage} from '@/pages/ConnectionsPage'
import {HistoryPage} from '@/pages/HistoryPage'
import {SettingsPage} from '@/pages/SettingsPage'

type Page = 'copy' | 'history' | 'connections' | 'settings'

const nav: {id: Page; label: string; icon: typeof CopyIcon}[] = [
  {id: 'copy', label: 'Copy', icon: CopyIcon},
  {id: 'history', label: 'Activity', icon: HistoryIcon},
  {id: 'connections', label: 'Connections', icon: DatabaseIcon},
  {id: 'settings', label: 'Settings', icon: SettingsIcon},
]

export default function App() {
  return (
    <ThemeProvider attribute="class" defaultTheme="system" enableSystem>
      <TooltipProvider>
        <Shell/>
        <Toaster position="bottom-right" richColors/>
      </TooltipProvider>
    </ThemeProvider>
  )
}

function Shell() {
  const [page, setPage] = useState<Page>('copy')
  const [connections, setConnections] = useState<Connection[]>([])
  const [jobId, setJobId] = useState<string | null>(null)

  const openJob = useCallback((id: string | null) => {
    setJobId(id)
    setPage('history')
  }, [])
  const openCopy = useCallback(() => setPage('copy'), [])

  const reload = useCallback(() => {
    api.listConnections().then(cs => setConnections(cs ?? []))
  }, [])
  useEffect(reload, [reload])

  return (
    <div className="flex h-screen bg-background text-foreground">
      <aside className="flex w-52 shrink-0 flex-col border-r bg-sidebar p-3">
        <div className="flex items-center gap-2 px-2 pb-4 pt-1 text-sm font-semibold">
          <div className="flex size-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
            <DatabaseIcon className="size-4"/>
          </div>
          DB Copier
        </div>
        <nav className="grid gap-1">
          {nav.map(({id, label, icon: Icon}) => (
            <button key={id} onClick={() => { setPage(id); if (id === 'history') setJobId(null) }}
                    className={cn('flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-muted-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground',
                      page === id && 'bg-sidebar-accent font-medium text-sidebar-accent-foreground')}>
              <Icon className="size-4"/> {label}
            </button>
          ))}
        </nav>
        <div className="mt-auto">
          <ThemeToggle/>
        </div>
      </aside>

      {/* Pages stay mounted so a running copy keeps its log while browsing other pages. */}
      <main className="min-w-0 flex-1 overflow-auto p-6">
        <div className={cn('h-full', page !== 'copy' && 'hidden')}><CopyPage connections={connections} onOpenJob={openJob}/></div>
        <div className={cn('h-full', page !== 'history' && 'hidden')}><HistoryPage jobId={jobId} onOpenJob={openJob} onOpenCopy={openCopy}/></div>
        <div className={cn(page !== 'connections' && 'hidden')}><ConnectionsPage connections={connections} reload={reload}/></div>
        <div className={cn(page !== 'settings' && 'hidden')}><SettingsPage/></div>
      </main>
    </div>
  )
}

function ThemeToggle() {
  const {resolvedTheme, setTheme} = useTheme()
  const dark = resolvedTheme === 'dark'
  return (
    <Button variant="ghost" size="sm" className="w-full justify-start text-muted-foreground"
            onClick={() => setTheme(dark ? 'light' : 'dark')}>
      {dark ? <SunIcon/> : <MoonIcon/>} {dark ? 'Light' : 'Dark'} mode
    </Button>
  )
}
