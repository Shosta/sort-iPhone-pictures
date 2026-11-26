// Prépare une release en LOCAL : met à jour package.json / lockfile / CHANGELOG.md, crée le commit « release » puis le tag annoté.
// Ne pousse jamais rien : le push du tag (qui déclenche la publication) reste une décision humaine.
//
//   node scripts/release.mjs 1.0.0-rc.1            (ou : npm run release -- 1.0.0-rc.1)
//   node scripts/release.mjs 1.0.0 --dry-run       affiche le changelog sans rien modifier
import { execFileSync } from 'node:child_process'
import { existsSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath, pathToFileURL } from 'node:url'
import { compareSemver, isPrerelease, parseCommits, parseSemver, previousTag, renderSection, setPackageVersion, updateChangelog } from './release-lib.mjs'

const root = resolve(dirname(fileURLToPath(import.meta.url)), '..', '..')
const electronDir = resolve(root, 'photosort-electron')

const git = (...args) => execFileSync('git', args, { cwd: root, encoding: 'utf8' }).trim()

export function release(version, { dryRun = false, today = new Date().toISOString().slice(0, 10) } = {}) {
  if (!parseSemver(version)) throw new Error(`« ${version} » n'est pas une version SemVer (1.0.0 ou 1.0.0-rc.1, sans préfixe « v »).`)
  const tag = `v${version}`
  const pkgPath = resolve(electronDir, 'package.json')
  const lockPath = resolve(electronDir, 'package-lock.json')
  const changelogPath = resolve(root, 'CHANGELOG.md')

  const current = JSON.parse(readFileSync(pkgPath, 'utf8')).version
  if (compareSemver(version, current) < 0) throw new Error(`La version ${version} est antérieure à la version actuelle (${current}).`)
  if (!dryRun && git('status', '--porcelain')) throw new Error('Le dépôt a des modifications non commitées : commitez ou remisez-les avant de publier.')
  const tags = git('tag', '--list', 'v*', '--merged', 'HEAD').split('\n').filter(Boolean)
  if (git('tag', '--list', tag)) throw new Error(`Le tag ${tag} existe déjà.`)

  const previous = previousTag(tags, version)
  const range = previous ? [`${previous}..HEAD`] : ['HEAD']
  const subjects = git('log', '--no-merges', '--format=%s', ...range).split('\n').filter(Boolean)
  const entries = parseCommits(subjects)
  console.log(`Release ${tag} (depuis ${previous ?? 'l\'origine'}, ${entries.length} commit(s) feat/fix)`)
  if (dryRun) {
    console.log(`\n${renderSection(version, today, entries)}`)
    return { tag, dryRun }
  }

  const existing = existsSync(changelogPath) ? readFileSync(changelogPath, 'utf8') : ''
  const changelog = updateChangelog(existing, version, today, entries)
  writeFileSync(pkgPath, setPackageVersion(readFileSync(pkgPath, 'utf8'), version))
  if (existsSync(lockPath)) writeFileSync(lockPath, setPackageVersion(readFileSync(lockPath, 'utf8'), version))
  writeFileSync(changelogPath, changelog)

  git('add', 'CHANGELOG.md', 'photosort-electron/package.json', 'photosort-electron/package-lock.json')
  git('commit', '-m', `release: ${tag}`)
  git('tag', '-a', tag, '-m', `Photosort ${version}${isPrerelease(version) ? ' (préversion)' : ''}`)
  console.log(`\nCommit « release: ${tag} » et tag ${tag} créés en local. Rien n'a été poussé.`)
  console.log(`Pour publier (après fusion sur main) : git push origin ${tag}`)
  return { tag, dryRun }
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const args = process.argv.slice(2)
  const version = args.find((a) => !a.startsWith('--'))
  try {
    if (!version) throw new Error('Usage : node scripts/release.mjs <version> [--dry-run]')
    release(version, { dryRun: args.includes('--dry-run') })
  } catch (error) {
    console.error(`Erreur : ${error.message}`)
    process.exit(1)
  }
}
