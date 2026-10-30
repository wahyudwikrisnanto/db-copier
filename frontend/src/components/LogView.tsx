import {useEffect, useRef} from 'react'
import {cn} from '@/lib/utils'

export const isErrorLine = (line: string) => line.includes('ERROR') || line.includes('error:')

// Monospace log pane. With follow, it stays scrolled to the newest line.
export function LogView({lines, follow, className}: {lines: string[]; follow?: boolean; className?: string}) {
  const end = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (follow) end.current?.scrollIntoView({block: 'end'})
  }, [lines, follow])

  return (
    <div className={cn('overflow-auto rounded-lg bg-muted/50 p-3 font-mono text-xs leading-relaxed', className)}>
      {lines.length === 0
        ? <span className="text-muted-foreground">Nothing yet.</span>
        : lines.map((line, i) => (
          <div key={i} className={cn('whitespace-pre-wrap break-all', isErrorLine(line) && 'text-destructive')}>{line}</div>
        ))}
      <div ref={end}/>
    </div>
  )
}
