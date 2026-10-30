import {BanIcon, CircleCheckIcon, CircleXIcon, Loader2Icon} from 'lucide-react'
import {Badge} from '@/components/ui/badge'
import {cn} from '@/lib/utils'
import {JobStatus} from '@/lib/api'

const labels: Record<JobStatus, string> = {
  running: 'Running',
  completed: 'Completed',
  failed: 'Failed',
  cancelled: 'Cancelled',
}

export function JobStatusIcon({status, className}: {status: JobStatus; className?: string}) {
  switch (status) {
    case 'running':
      return <Loader2Icon className={cn('animate-spin text-status-running', className)}/>
    case 'completed':
      return <CircleCheckIcon className={cn('text-status-completed', className)}/>
    case 'failed':
      return <CircleXIcon className={cn('text-status-failed', className)}/>
    case 'cancelled':
      return <BanIcon className={cn('text-status-cancelled', className)}/>
  }
}

export function JobStatusBadge({status}: {status: JobStatus}) {
  switch (status) {
    case 'running':
      return <Badge className="bg-status-running text-primary-foreground"><Loader2Icon className="animate-spin"/> {labels.running}</Badge>
    case 'completed':
      return <Badge className="border-success-border bg-success-bg text-success">{labels.completed}</Badge>
    case 'failed':
      return <Badge className="border-danger-border bg-danger-bg text-danger">{labels.failed}</Badge>
    case 'cancelled':
      return <Badge className="border-border bg-muted text-status-cancelled">{labels.cancelled}</Badge>
  }
}

export const jobStatusLabel = (s: JobStatus) => labels[s]
