import type { Duplicate, Entry, Issue, Plan } from './types'

interface PlanCounts {
  entries: number
  skipped: number
  duplicates: number
}

/**
 * The sidecar sends a plan as planChunk events (a batch of entries, skipped or
 * duplicates each) followed by a small plan event with the destination and the
 * counts. The assembler appends each chunk as it arrives, so the renderer never
 * parses or clones one multi-megabyte message, and hands back the whole plan
 * when the closing event shows every chunk made it.
 */
export function createPlanAssembler(): {
  add: (chunk: { entries?: Entry[]; skipped?: Issue[]; duplicates?: Duplicate[] }) => void
  finish: (event: any) => any
} {
  const entries: Entry[] = []
  const skipped: Issue[] = []
  const duplicates: Duplicate[] = []
  const append = <T>(into: T[], batch: T[] | undefined): void => {
    if (!batch) return
    for (const item of batch) into.push(item)
  }
  return {
    add(chunk) {
      append(entries, chunk.entries)
      append(skipped, chunk.skipped)
      append(duplicates, chunk.duplicates)
    },
    finish(event) {
      // A plan that still carries its own entries (older sidecar) is complete as is.
      if (event.plan?.entries) return event
      const counts: PlanCounts | undefined = event.counts
      if (!counts || counts.entries !== entries.length || counts.skipped !== skipped.length || counts.duplicates !== duplicates.length) {
        return { id: event.id, type: 'error', message: 'Plan incomplet : des morceaux du plan se sont perdus entre le moteur et l’interface.' }
      }
      const plan: Plan = { destination: event.plan?.destination ?? '', entries, skipped, duplicates }
      return { id: event.id, type: 'plan', plan }
    }
  }
}
