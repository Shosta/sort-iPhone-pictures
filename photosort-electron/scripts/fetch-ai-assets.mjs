// Telecharge les modeles IA (hors ligne) et ONNX Runtime pour la plateforme cible,
// verifie leur sha256 et les place dans ai-assets/ (ignore par git).
// Variables optionnelles : GOOS (darwin|windows|linux), GOARCH (amd64|arm64).
import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { readZipEntry } from './zip-entry.mjs'

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

// ONNX Runtime : les binaires ne sont plus publies sur GitHub Releases ; onnxruntime.ai (Install) renvoie vers
// le paquet NuGet Microsoft.ML.OnnxRuntime, un zip unique qui contient la bibliotheque de chaque plateforme.
// Pas de build macOS Intel. Garder en phase avec src/main/assets.ts.
const ORT_URL = `https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/${ORT}/microsoft.ml.onnxruntime.${ORT}.nupkg`
const ORT_SHA256 = '6871ccbc97f3d5bdb4095a1dcec33aa78d56f6ed8de5980ef5fa1d9f2e7c679a'
const runtimes = {
  'darwin-arm64': { entry: 'runtimes/osx-arm64/native/libonnxruntime.dylib', dest: 'libonnxruntime.dylib' },
  'linux-amd64': { entry: 'runtimes/linux-x64/native/libonnxruntime.so', dest: 'libonnxruntime.so' },
  'linux-arm64': { entry: 'runtimes/linux-arm64/native/libonnxruntime.so', dest: 'libonnxruntime.so' },
  'windows-amd64': { entry: 'runtimes/win-x64/native/onnxruntime.dll', dest: 'onnxruntime.dll' },
  'windows-arm64': { entry: 'runtimes/win-arm64/native/onnxruntime.dll', dest: 'onnxruntime.dll' }
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

mkdirSync(out, { recursive: true })
for (const model of models) download(model.url, join(out, model.file), model.sha256)

const runtime = runtimes[`${goos}-${goarch}`]
if (!runtime) {
  console.warn(`ONNX Runtime ${ORT} n'est pas fourni pour ${goos}/${goarch} : l'etiquetage IA sera desactive.`)
} else {
  const work = mkdtempSync(join(tmpdir(), 'ort-'))
  try {
    const archive = join(work, 'ort.nupkg')
    download(ORT_URL, archive, ORT_SHA256)
    const lib = readZipEntry(readFileSync(archive), runtime.entry)
    if (!lib) throw new Error(`${runtime.entry} introuvable dans l archive`)
    writeFileSync(join(out, runtime.dest), lib)
  } finally {
    rmSync(work, { recursive: true, force: true })
  }
}
console.log('modeles IA ->', out)
