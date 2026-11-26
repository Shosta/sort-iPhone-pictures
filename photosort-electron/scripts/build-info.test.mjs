import assert from 'node:assert/strict'
import { describe, it } from 'node:test'
import { resolveCommit, shortSha } from './build-info.mjs'

describe('shortSha', () => {
  it('shortens and lowercases a valid SHA', () => {
    assert.equal(shortSha('B89B82E1234567890abcdef1234567890abcdef1\n'), 'b89b82e')
  })
  it('rejects anything else', () => {
    for (const bad of ['', undefined, null, 'xyz', 'abc12', 'not-a-sha-at-all']) assert.equal(shortSha(bad), '')
  })
})

describe('resolveCommit', () => {
  const git = () => 'aaaaaaa1111111'
  it('prefers PHOTOSORT_COMMIT, then GITHUB_SHA, then git', () => {
    assert.equal(resolveCommit({ env: { PHOTOSORT_COMMIT: 'bbbbbbb2', GITHUB_SHA: 'ccccccc3' }, git }), 'bbbbbbb')
    assert.equal(resolveCommit({ env: { GITHUB_SHA: 'ccccccc3' }, git }), 'ccccccc')
    assert.equal(resolveCommit({ env: {}, git }), 'aaaaaaa')
  })
  it('falls back to an empty string without git', () => {
    assert.equal(resolveCommit({ env: {}, git: () => '' }), '')
  })
})
