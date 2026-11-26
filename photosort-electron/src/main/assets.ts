import { net } from 'electron'
import { createHash } from 'node:crypto'
import { existsSync, mkdirSync, mkdtempSync, readFileSync, renameSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { readZipEntry } from '../../scripts/zip-entry.mjs'

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

// onnxruntime.ai (Install) distributes the runtime as the NuGet package Microsoft.ML.OnnxRuntime: one zip holding
// every platform's library (GitHub Releases no longer hosts the binaries). No macOS Intel build.
// Keep in sync with scripts/fetch-ai-assets.mjs.
const ORT_URL = `https://api.nuget.org/v3-flatcontainer/microsoft.ml.onnxruntime/${ORT}/microsoft.ml.onnxruntime.${ORT}.nupkg`
const ORT_SHA256 = '6871ccbc97f3d5bdb4095a1dcec33aa78d56f6ed8de5980ef5fa1d9f2e7c679a'

interface Runtime {
  entry: string
  dest: string
}

const runtimes: Record<string, Runtime> = {
  'darwin-arm64': { entry: 'runtimes/osx-arm64/native/libonnxruntime.dylib', dest: 'libonnxruntime.dylib' },
  'linux-x64': { entry: 'runtimes/linux-x64/native/libonnxruntime.so', dest: 'libonnxruntime.so' },
  'linux-arm64': { entry: 'runtimes/linux-arm64/native/libonnxruntime.so', dest: 'libonnxruntime.so' },
  'win32-x64': { entry: 'runtimes/win-x64/native/onnxruntime.dll', dest: 'onnxruntime.dll' },
  'win32-arm64': { entry: 'runtimes/win-arm64/native/onnxruntime.dll', dest: 'onnxruntime.dll' }
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
  if (existsSync(join(dir, runtime.dest)) && existsSync(marker) && readFileSync(marker, 'utf8') === ORT_SHA256) return

  const work = mkdtempSync(join(tmpdir(), 'ort-'))
  try {
    const archive = join(work, 'ort.nupkg')
    await download({ file: `ONNX Runtime ${ORT}`, url: ORT_URL, sha256: ORT_SHA256 }, archive, report)
    const lib = readZipEntry(readFileSync(archive), runtime.entry)
    if (!lib) throw new AssetError('ONNX Runtime', ORT_URL, `${runtime.entry} not found in archive`)
    writeFileSync(join(dir, runtime.dest), lib)
    writeFileSync(marker, ORT_SHA256)
  } finally {
    rmSync(work, { recursive: true, force: true })
  }
}
