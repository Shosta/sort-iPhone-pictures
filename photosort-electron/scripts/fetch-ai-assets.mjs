// Telecharge les modeles IA (hors ligne) et ONNX Runtime pour la plateforme cible,
// verifie leur sha256 et les place dans ai-assets/ (ignore par git).
// Variables optionnelles : GOOS (darwin|windows|linux), GOARCH (amd64|arm64).
import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { copyFileSync, existsSync, mkdirSync, mkdtempSync, readdirSync, readFileSync, rmSync, statSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const ORT = '1.31.1'
const here = dirname(fileURLToPath(import.meta.url))
const out = resolve(here, '..', 'ai-assets')

const goos = process.env.GOOS ?? { darwin: 'darwin', win32: 'windows', linux: 'linux' }[process.platform]
const goarch = process.env.GOARCH ?? { x64: 'amd64', arm64: 'arm64' }[process.arch]

// Modeles communs (MIT / Apache-2.0).
const models = [
  {
    file: 'places.onnx', // Places365 ResNet18 (MIT)
    url: 'https://huggingface.co/mlboydaisuke/Places365-ResNet18-OpenVINO/resolve/main/places_fp16.onnx',
    sha256: 'c076eeab9d1cc9679ffa08f93c2e3cb5c74734577b2c7221a60f53c57567042a'
  },
  {
    file: 'categories.txt',
    url: 'https://raw.githubusercontent.com/csailvision/places365/master/categories_places365.txt',
    sha256: '2affba635eb657e7ca95f4e6cc69bd9fac29ef4c32aeb83cafdfcd06ec6a1ea6'
  },
  {
    file: 'yunet.onnx', // YuNet, detection de visages (MIT)
    url: 'https://github.com/opencv/opencv_zoo/raw/main/models/face_detection_yunet/face_detection_yunet_2023mar.onnx',
    sha256: '8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4'
  }
]

// Bibliotheque ONNX Runtime par plateforme (pas de build macOS Intel en 1.31).
const runtimes = {
  'darwin-arm64': { name: 'osx-arm64', ext: 'tgz', lib: /^libonnxruntime\.\d[\d.]*\.dylib$/, dest: 'libonnxruntime.dylib', sha256: '2e1344a1926ee50eb25b38153cb6992c3567515aef0e677a1a520a1798896d6f' },
  'linux-amd64': { name: 'linux-x64', ext: 'tgz', lib: /^libonnxruntime\.so\.\d[\d.]*$/, dest: 'libonnxruntime.so', sha256: 'c849e36a1ef3e22d4ff217b9d1ce4a3cf5b9467b975610913ae959ad5b16bdc9' },
  'linux-arm64': { name: 'linux-aarch64', ext: 'tgz', lib: /^libonnxruntime\.so\.\d[\d.]*$/, dest: 'libonnxruntime.so', sha256: '5317505e2da99be65f8e2775ba3c2afd6d2961fd78aba1bd8f56159f48820c78' },
  'windows-amd64': { name: 'win-x64', ext: 'zip', lib: /^onnxruntime\.dll$/, dest: 'onnxruntime.dll', sha256: '5869e26bb25314375f2d5ef1877e3ea06b1543d6c710e6e4a1ec2f7177ab4b2c' },
  'windows-arm64': { name: 'win-arm64', ext: 'zip', lib: /^onnxruntime\.dll$/, dest: 'onnxruntime.dll', sha256: '817f4129073f84f81cde3ff92b1a4e06d007691e7208deed08a8b6d6df143f7f' }
}

const sha = (file) => createHash('sha256').update(readFileSync(file)).digest('hex')

function download(url, dest, expected) {
  if (existsSync(dest) && sha(dest) === expected) return
  console.log('telechargement', url)
  // curl (macOS, Linux, Windows 10+) utilise le magasin de certificats du systeme,
  // utile derriere un proxy d'entreprise.
  execFileSync('curl', ['-fsSL', '-o', dest, url], { stdio: 'inherit' })
  const actual = sha(dest)
  if (actual !== expected) {
    rmSync(dest)
    throw new Error(`${url}: sha256 inattendu (${actual})`)
  }
}

function findFile(dir, pattern) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry)
    if (statSync(path).isDirectory()) {
      const found = findFile(path, pattern)
      if (found) return found
    } else if (pattern.test(entry)) return path
  }
  return null
}

mkdirSync(out, { recursive: true })
for (const model of models) download(model.url, join(out, model.file), model.sha256)

const runtime = runtimes[`${goos}-${goarch}`]
if (!runtime) {
  console.warn(`ONNX Runtime ${ORT} n'est pas fourni pour ${goos}/${goarch} : l'etiquetage IA sera desactive.`)
} else {
  const work = mkdtempSync(join(tmpdir(), 'ort-'))
  try {
    const archive = join(work, `ort.${runtime.ext}`)
    download(`https://github.com/microsoft/onnxruntime/releases/download/v${ORT}/onnxruntime-${runtime.name}-${ORT}.${runtime.ext}`, archive, runtime.sha256)
    // bsdtar (macOS, Linux, Windows 10+) lit aussi les .zip.
    execFileSync('tar', ['-xf', archive, '-C', work])
    const lib = findFile(work, runtime.lib)
    if (!lib) throw new Error('bibliotheque ONNX Runtime introuvable dans l archive')
    copyFileSync(lib, join(out, runtime.dest))
  } finally {
    rmSync(work, { recursive: true, force: true })
  }
}
console.log('modeles IA ->', out)
