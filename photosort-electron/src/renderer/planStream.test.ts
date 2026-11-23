import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { createPlanAssembler } from './planStream.ts'

const entry = (n: number) => ({ source: `/src/${n}.jpg`, target: `/dest/2026/${n}.jpg` })

describe('createPlanAssembler', () => {
  it('assembles the chunks, in order, into the plan the closing event announces', () => {
    const assembler = createPlanAssembler()
    assembler.add({ entries: [entry(1), entry(2)] })
    assembler.add({ entries: [entry(3)] })
    assembler.add({ skipped: [{ path: '/src/x', reason: 'unreadable' }] })
    assembler.add({ duplicates: [{ path: '/src/d', of: '/src/1.jpg', inDestination: false }] })
    const event = assembler.finish({ id: '7', type: 'plan', plan: { destination: '/dest' }, counts: { entries: 3, skipped: 1, duplicates: 1 } })
    assert.equal(event.type, 'plan')
    assert.equal(event.id, '7')
    assert.equal(event.plan.destination, '/dest')
    assert.deepEqual(event.plan.entries, [entry(1), entry(2), entry(3)])
    assert.equal(event.plan.skipped.length, 1)
    assert.equal(event.plan.duplicates[0].of, '/src/1.jpg')
  })

  it('handles an empty plan', () => {
    const event = createPlanAssembler().finish({ id: '1', type: 'plan', plan: { destination: '/dest' }, counts: { entries: 0, skipped: 0, duplicates: 0 } })
    assert.deepEqual(event.plan, { destination: '/dest', entries: [], skipped: [], duplicates: [] })
  })

  it('turns a plan with a lost chunk into an error instead of a partial plan', () => {
    const assembler = createPlanAssembler()
    assembler.add({ entries: [entry(1)] })
    const event = assembler.finish({ id: '4', type: 'plan', plan: { destination: '/dest' }, counts: { entries: 2, skipped: 0, duplicates: 0 } })
    assert.equal(event.type, 'error')
    assert.equal(event.id, '4')
    assert.ok(event.message)
  })

  it('rejects a closing event without counts', () => {
    const event = createPlanAssembler().finish({ id: '5', type: 'plan', plan: { destination: '/dest' } })
    assert.equal(event.type, 'error')
  })

  it('passes a plan that carries its own entries through unchanged', () => {
    const whole = { id: '6', type: 'plan', plan: { destination: '/d', entries: [entry(1)], skipped: [], duplicates: [] } }
    assert.equal(createPlanAssembler().finish(whole), whole)
  })

  it('appends large chunks without exhausting the call stack', () => {
    const assembler = createPlanAssembler()
    const batch = Array.from({ length: 150_000 }, (_, i) => entry(i))
    assembler.add({ entries: batch })
    const event = assembler.finish({ id: '8', type: 'plan', plan: { destination: '/d' }, counts: { entries: 150_000, skipped: 0, duplicates: 0 } })
    assert.equal(event.plan.entries.length, 150_000)
  })
})
