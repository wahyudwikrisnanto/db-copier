import {cn} from '@/lib/utils'
import {connectionColors} from '@/lib/api'

export function ColorDot({color, className}: {color: string; className?: string}) {
  return <span className={cn('inline-block size-2 shrink-0 rounded-full', connectionColors[color] ?? 'bg-muted-foreground/40', className)}/>
}
