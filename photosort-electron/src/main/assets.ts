import { net } from 'electron'
import { createHash } from 'node:crypto'
import { execFileSync } from 'node:child_process'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, readdirSync, renameSync, rmSync, statSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'

const ORT = '1.31.1'

interface Asset {
  file: string
  url: string
  sha256: string
}

const models: Asset[] = [
  {
    file: 'places.onnx',
    url: 'https://huggingface.co/mlboydaisuke/Places365-ResNet18-OpenVINO/resolve/main/places_fp16.onnx',
    sha256: 'c076eeab9d1cc9679ffa08f93c2e3cb5c74734577b2c7221a60f53c57567042a'
  },
  {
    file: 'categories.txt',
    url: 'https://raw.githubusercontent.com/csailvision/places365/master/categories_places365.txt',
    sha256: '2affba635eb657e7ca95f4e6cc69bd9fac29ef4c32aeb83cafdfcd06ec6a1ea6'
  },
  {
    file: 'yunet.onnx',
    url: 'https://github.com/opencv/opencv_zoo/raw/main/models/face_detection_yunet/face_detection_yunet_2023mar.onnx',
    sha256: '8f2383e4dd3cfbb4553ea8718107fc0423210dc964f9f4280604804ed2552fa4'
  }
]

interface Runtime {
  name: string
  ext: string
  lib: RegExp
  dest: string
  sha256: string
}

const runtimes: Record<string, Runtime> = {
  'darwin-arm64': { name: 'osx-arm64', ext: 'tgz', lib: /^libonnxruntime\.\d[\d.]*\.dylib$/, dest: 'libonnxruntime.dylib', sha256: '2e1344a1926ee50eb25b38153cb6992c3567515aef0e677a1a520a1798896d6f' },
  'linux-x64': { name: 'linux-x64', ext: 'tgz', lib: /^libonnxruntime\.so\.\d[\d.]*$/, dest: 'libonnxruntime.so', sha256: 'c849e36a1ef3e22d4ff217b9d1ce4a3cf5b9467b975610913ae959ad5b16bdc9' },
  'linux-arm64': { name: 'linux-aarch64', ext: 'tgz', lib: /^libonnxruntime\.so\.\d[\d.]*$/, dest: 'libonnxruntime.so', sha256: '5317505e2da99be65f8e2775ba3c2afd6d2961fd78aba1bd8f56159f48820c78' },
  'win32-x64': { name: 'win-x64', ext: 'zip', lib: /^onnxruntime\.dll$/, dest: 'onnxruntime.dll', sha256: '5869e26bb25314375f2d5ef1877e3ea06b1543d6c710e6e4a1ec2f7177ab4b2c' },
  'win32-arm64': { name: 'win-arm64', ext: 'zip', lib: /^onnxruntime\.dll$/, dest: 'onnxruntime.dll', sha256: '817f4129073f84f81cde3ff92b1a4e06d007691e7208deed08a8b6d6df143f7f' }
}

export interface AssetsState {
  state: 'checking' | 'downloading' | 'ready' | 'error'
  file?: string
  index?: number
  total?: number
  received?: number
  size?: number
  error?: { file: string; url: string; reason: string; dir: string }
}

type Report = (patch: Partial<AssetsState>) => void

class AssetError extends Error {
  constructor(
    readonly file: string,
    readonly url: string,
    readonly reason: string
  ) {
    super(reason)
  }
}

const sha = (file: string): string => createHash('sha256').update(readFileSync(file)).digest('hex')

async function download(asset: { file: string; url: string; sha256: string }, dest: string, report: Report): Promise<void> {
  if (existsSync(dest) && sha(dest) === asset.sha256) return
  report({ file: asset.file, received: 0, size: undefined })
  let res: Response
  try {
    // Chromium network stack: honours the system certificate store and proxy (corporate TLS interception).
    res = await net.fetch(asset.url, { redirect: 'follow' })
  } catch (err) {
    const cause = (err as { cause?: { code?: string; message?: string } }).cause
    throw new AssetError(asset.file, asset.url, `network error: ${cause?.code ?? cause?.message ?? (err as Error).message}`)
  }
  if (!res.ok || !res.body) throw new AssetError(asset.file, asset.url, `HTTP ${res.status} ${res.statusText}`.trim())
  const size = Number(res.headers.get('content-length')) || undefined
  report({ size })
  const chunks: Buffer[] = []
  let received = 0
  try {
    for await (const chunk of res.body as unknown as AsyncIterable<Uint8Array>) {
      chunks.push(Buffer.from(chunk))
      received += chunk.length
      report({ received })
    }
  } catch (err) {
    throw new AssetError(asset.file, asset.url, `download interrupted: ${(err as Error).message}`)
  }
  const tmp = `${dest}.part`
  writeFileSync(tmp, Buffer.concat(chunks))
  const actual = sha(tmp)
  if (actual !== asset.sha256) {
    rmSync(tmp)
    throw new AssetError(asset.file, asset.url, `checksum mismatch (got ${actual.slice(0, 12)}…, expected ${asset.sha256.slice(0, 12)}…)`)
  }
  renameSync(tmp, dest)
}

function findFile(dir: string, pattern: RegExp): string | null {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry)
    if (statSync(path).isDirectory()) {
      const found = findFile(path, pattern)
      if (found) return found
    } else if (pattern.test(entry)) return path
  }
  return null
}

export function runtimeLibName(): string {
  return { darwin: 'libonnxruntime.dylib', win32: 'onnxruntime.dll' }[process.platform as string] ?? 'libonnxruntime.so'
}

export function assetsComplete(dir: string): boolean {
  return [runtimeLibName(), ...models.map((m) => m.file)].every((f) => existsSync(join(dir, f)))
}

// Downloads (or refreshes, when a pinned checksum changed) the models and ONNX Runtime into dir.
export async function ensureAssets(dir: string, onState: (state: AssetsState) => void): Promise<void> {
  const total = models.length + 1
  let state: AssetsState = { state: 'downloading', index: 0, total }
  const report: Report = (patch) => {
    state = { ...state, ...patch }
    onState(state)
  }
  report({})
  try {
    mkdirSync(dir, { recursive: true })
    let index = 0
    for (const model of models) {
      report({ index: ++index })
      await download(model, join(dir, model.file), report)
    }
    report({ index: total })
    await ensureRuntime(dir, report)
    onState({ state: 'ready' })
  } catch (err) {
    const e = err instanceof AssetError ? err : new AssetError(state.file ?? '', '', (err as Error).message)
    onState({ state: 'error', error: { file: e.file, url: e.url, reason: e.reason, dir } })
  }
}

async function ensureRuntime(dir: string, report: Report): Promise<void> {
  const runtime = runtimes[`${process.platform}-${process.arch}`]
  if (!runtime) throw new AssetError('ONNX Runtime', '', `no ONNX Runtime ${ORT} build for ${process.platform}/${process.arch}`)
  const marker = join(dir, `${runtime.dest}.sha256`)
  if (existsSync(join(dir, runtime.dest)) && existsSync(marker) && readFileSync(marker, 'utf8') === runtime.sha256) return

  const work = mkdtempSync(join(tmpdir(), 'ort-'))
  try {
    const archive = join(work, `ort.${runtime.ext}`)
    const url = `https://github.com/microsoft/onnxruntime/releases/download/v${ORT}/onnxruntime-${runtime.name}-${ORT}.${runtime.ext}`
    await download({ file: `ONNX Runtime ${ORT}`, url, sha256: runtime.sha256 }, archive, report)
    execFileSync('tar', ['-xf', archive, '-C', work])
    const lib = findFile(work, runtime.lib)
    if (!lib) throw new AssetError('ONNX Runtime', url, 'library not found in archive')
    writeFileSync(join(dir, runtime.dest), readFileSync(lib))
    writeFileSync(marker, runtime.sha256)
  } finally {
    rmSync(work, { recursive: true, force: true })
  }
}
