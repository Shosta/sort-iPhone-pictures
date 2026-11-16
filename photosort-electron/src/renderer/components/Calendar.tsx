import { motion } from 'framer-motion'
import { useMemo, useState } from 'react'

const pad = (n: number): string => String(n).padStart(2, '0')

/** Local calendar day as YYYY-MM-DD, the format the engine expects. */
export const isoDay = (date: Date): string => `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`

export const addDays = (date: Date, days: number): Date => new Date(date.getFullYear(), date.getMonth(), date.getDate() + days)

/** The Monday of the week holding `date`, at local midnight. */
export const mondayOf = (date: Date): Date => addDays(date, -((date.getDay() + 6) % 7))

/** ISO 8601 week of a Monday: the Thursday of the week decides the year. */
export function isoWeek(monday: Date): { year: number; week: number } {
  const year = addDays(monday, 3).getFullYear()
  const first = mondayOf(new Date(year, 0, 4))
  return { year, week: Math.round((monday.getTime() - first.getTime()) / (7 * 86_400_000)) + 1 }
}

const ROWS = 6

/** Six Monday-first weeks covering the month: the height never jumps between months. */
export function monthWeeks(month: Date): Date[][] {
  const start = mondayOf(new Date(month.getFullYear(), month.getMonth(), 1))
  return Array.from({ length: ROWS }, (_, week) => Array.from({ length: 7 }, (_, day) => addDays(start, week * 7 + day)))
}

interface Props {
  locale: string
  /** Any day of the month shown. */
  month: Date
  /** Photos per day (YYYY-MM-DD). */
  counts: Record<string, number>
  /** Monday (YYYY-MM-DD) of the picked week. */
  selected: string | null
  prevLabel: string
  nextLabel: string
  onMonth: (delta: number) => void
  onPick: (monday: Date) => void
}

export function Calendar({ locale, month, counts, selected, prevLabel, nextLabel, onMonth, onPick }: Props): JSX.Element {
  const [direction, setDirection] = useState(1)
  const weeks = useMemo(() => monthWeeks(month), [month])
  const heads = useMemo(() => {
    const narrow = new Intl.DateTimeFormat(locale, { weekday: 'narrow' })
    const long = new Intl.DateTimeFormat(locale, { weekday: 'long' })
    // 1 January 2024 is a Monday.
    return Array.from({ length: 7 }, (_, index) => ({ short: narrow.format(new Date(2024, 0, 1 + index)), long: long.format(new Date(2024, 0, 1 + index)) }))
  }, [locale])
  const title = new Intl.DateTimeFormat(locale, { month: 'long', year: 'numeric' }).format(month)
  const today = isoDay(new Date())
  const key = `${month.getFullYear()}-${month.getMonth()}`

  const go = (delta: number): void => {
    setDirection(delta)
    onMonth(delta)
  }

  return (
    <div className="cal">
      <div className="cal-head">
        <button className="cal-nav" aria-label={prevLabel} onClick={() => go(-1)}>
          ‹
        </button>
        <strong className="cal-title">{title}</strong>
        <button className="cal-nav" aria-label={nextLabel} onClick={() => go(1)}>
          ›
        </button>
      </div>
      <div className="cal-row cal-heads" aria-hidden>
        <span />
        {heads.map((head, index) => (
          <span key={index} title={head.long}>
            {head.short}
          </span>
        ))}
      </div>
      <motion.div key={key} className="cal-weeks" initial={{ opacity: 0, x: direction * 22 }} animate={{ opacity: 1, x: 0 }} transition={{ duration: 0.28, ease: 'easeOut' }}>
        {weeks.map((week) => {
          const monday = isoDay(week[0])
          const picked = selected === monday
          return (
            <div key={monday} className={`cal-row cal-week${picked ? ' picked' : ''}`}>
              {picked && <motion.i className="cal-tape" initial={{ scaleX: 0.6, opacity: 0 }} animate={{ scaleX: 1, opacity: 1 }} transition={{ type: 'spring', stiffness: 420, damping: 30 }} />}
              <span className="cal-num">{isoWeek(week[0]).week}</span>
              {week.map((day) => {
                const iso = isoDay(day)
                const count = counts[iso] ?? 0
                const classes = ['cal-day']
                if (day.getMonth() !== month.getMonth()) classes.push('out')
                if (iso === today) classes.push('today')
                if (count > 0) classes.push('has')
                return (
                  <button key={iso} className={classes.join(' ')} onClick={() => onPick(week[0])}>
                    {day.getDate()}
                    {count > 0 && <em>{count}</em>}
                  </button>
                )
              })}
            </div>
          )
        })}
      </motion.div>
    </div>
  )
}
