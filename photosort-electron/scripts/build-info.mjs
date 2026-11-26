// Empreinte du commit injectée au build (SHA court affiché à côté de la version SemVer).
// Ordre : PHOTOSORT_COMMIT, GITHUB_SHA, puis `git rev-parse`. Sans git (archive source), renvoie '' : l'appli affiche alors la version seule.
import { execFileSync } from 'node:child_process'

export function shortSha(raw) {
  const sha = String(raw ?? '').trim()
  return /^[0-9a-f]{7,40}$/i.test(sha) ? sha.slice(0, 7).toLowerCase() : ''
}

function gitHead(cwd) {
  try {
    return execFileSync('git', ['rev-parse', 'HEAD'], { cwd, stdio: ['ignore', 'pipe', 'ignore'], encoding: 'utf8' })
  } catch {
    return ''
  }
}

export function resolveCommit({ env = process.env, cwd = process.cwd(), git = gitHead } = {}) {
  return shortSha(env.PHOTOSORT_COMMIT) || shortSha(env.GITHUB_SHA) || shortSha(git(cwd))
}
