// Extrait une entree d'une archive zip (ex. un .nupkg) sans outil externe :
// GNU tar (Linux, Git Bash) ne lit pas les zip, bsdtar si.
import { inflateRawSync } from 'node:zlib'

// Retourne le contenu de l'entree `name` (Buffer), ou null si elle est absente.
export function readZipEntry(zip, name) {
  let eocd = -1
  for (let i = zip.length - 22; i >= Math.max(0, zip.length - 22 - 0xffff); i--) {
    if (zip.readUInt32LE(i) === 0x06054b50) {
      eocd = i
      break
    }
  }
  if (eocd < 0) throw new Error('archive zip invalide (fin de repertoire central introuvable)')
  const count = zip.readUInt16LE(eocd + 10)
  let pos = zip.readUInt32LE(eocd + 16)
  for (let n = 0; n < count; n++) {
    if (zip.readUInt32LE(pos) !== 0x02014b50) throw new Error('archive zip invalide (repertoire central)')
    const method = zip.readUInt16LE(pos + 10)
    const compressed = zip.readUInt32LE(pos + 20)
    const nameLen = zip.readUInt16LE(pos + 28)
    const extraLen = zip.readUInt16LE(pos + 30)
    const commentLen = zip.readUInt16LE(pos + 32)
    const local = zip.readUInt32LE(pos + 42)
    const entry = zip.toString('utf8', pos + 46, pos + 46 + nameLen)
    if (entry === name) {
      if (zip.readUInt32LE(local) !== 0x04034b50) throw new Error('archive zip invalide (entree locale)')
      const start = local + 30 + zip.readUInt16LE(local + 26) + zip.readUInt16LE(local + 28)
      const data = zip.subarray(start, start + compressed)
      if (method === 0) return Buffer.from(data)
      if (method === 8) return inflateRawSync(data)
      throw new Error(`methode de compression zip non geree (${method})`)
    }
    pos += 46 + nameLen + extraLen + commentLen
  }
  return null
}
