const timeFmt = new Intl.DateTimeFormat(undefined, {hour: '2-digit', minute: '2-digit'})
const dateTimeFmt = new Intl.DateTimeFormat(undefined, {dateStyle: 'medium', timeStyle: 'medium'})
const dayFmt = new Intl.DateTimeFormat(undefined, {weekday: 'long', month: 'long', day: 'numeric', year: 'numeric'})

const startOfDay = (d: Date) => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime()

export const formatTime = (iso: string) => timeFmt.format(new Date(iso))
export const formatDateTime = (iso: string) => dateTimeFmt.format(new Date(iso))

// "Today", "Yesterday" or the full date, for grouping lists by day.
export function formatDay(iso: string): string {
  const d = new Date(iso)
  const days = Math.round((startOfDay(new Date()) - startOfDay(d)) / 86_400_000)
  if (days === 0) return 'Today'
  if (days === 1) return 'Yesterday'
  return dayFmt.format(d)
}
