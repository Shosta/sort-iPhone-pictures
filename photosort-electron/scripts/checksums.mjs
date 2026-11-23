// Empreintes SHA-256 des installeurs, au format de `sha256sum` / `shasum -a 256`.
//
//   node scripts/checksums.mjs [dossier]          ecrit <dossier>/SHA256SUMS (defaut : release)
//   node scripts/checksums.mjs [dossier] --check  verifie les fichiers contre SHA256SUMS
//
// Verification cote utilisateur :
//   macOS : shasum -a 256 -c SHA256SUMS --ignore-missing
//   Linux : sha256sum -c SHA256SUMS --ignore-missing
import { createHash } from 'node:crypto'
import { createReadStream, existsSync, readdirSync, readFileSync, statSync, writeFileSync } from 'node:fs'
import { basename, join, resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

export const MANIFEST = 'SHA256SUMS'
const ARTIFACT = /\.(dmg|exe|AppImage|deb|zip)$/

export function sha256(file) {
  return new Promise((done, fail) => {
    const hash = createHash('sha256')
    createReadStream(file)
      .on('data', (chunk) => hash.update(chunk))
      .on('error', fail)
      .on('end', () => done(hash.digest('hex')))
  })
}

export function listArtifacts(dir) {
  return readdirSync(dir)
    .filter((name) => ARTIFACT.test(name) && statSync(join(dir, name)).isFile())
    .sort()
}

export async function writeManifest(dir) {
  const names = listArtifacts(dir)
  if (names.length === 0) throw new Error(`aucun installeur (.dmg .exe .AppImage .deb .zip) dans ${dir}`)
  const lines = []
  for (const name of names) lines.push(`${await sha256(join(dir, name))}  ${name}`)
  writeFileSync(join(dir, MANIFEST), lines.join('\n') + '\n')
  return lines
}

// Retourne la liste des problemes ; vide quand tout correspond.
export async function verifyManifest(dir) {
  const manifest = join(dir, MANIFEST)
  if (!existsSync(manifest)) return [`${MANIFEST} introuvable dans ${dir}`]
  const problems = []
  const listed = new Set()
  for (const line of readFileSync(manifest, 'utf8').split('\n')) {
    if (line.trim() === '') continue
    const match = /^([0-9a-f]{64}) [ *](.+)$/.exec(line)
    if (!match || match[2] !== basename(match[2])) {
      problems.push(`ligne invalide : ${line}`)
      continue
    }
    const [, expected, name] = match
    listed.add(name)
    if (!existsSync(join(dir, name))) problems.push(`${name} : fichier manquant`)
    else if ((await sha256(join(dir, name))) !== expected) problems.push(`${name} : empreinte differente`)
  }
  for (const name of listArtifacts(dir)) if (!listed.has(name)) problems.push(`${name} : absent de ${MANIFEST}`)
  return problems
}

async function main() {
  const args = process.argv.slice(2)
  const check = args.includes('--check')
  const dir = resolve(args.find((a) => !a.startsWith('--')) ?? 'release')
  if (check) {
    const problems = await verifyManifest(dir)
    if (problems.length > 0) {
      for (const problem of problems) console.error(`ECHEC ${problem}`)
      process.exit(1)
    }
    console.log(`OK : ${MANIFEST} correspond aux installeurs de ${dir}`)
    return
  }
  for (const line of await writeManifest(dir)) console.log(line)
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  main().catch((error) => {
    console.error(error.message)
    process.exit(1)
  })
}
