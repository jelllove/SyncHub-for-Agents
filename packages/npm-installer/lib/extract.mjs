import { createWriteStream } from 'node:fs'
import { mkdir } from 'node:fs/promises'
import path from 'node:path'
import { pipeline } from 'node:stream/promises'
import yauzl from 'yauzl'
import * as tar from 'tar'
import { safeRelativePath } from './platform.mjs'

export async function extractArchive(file, destination, asset, { maxBytes = 512 * 1024 * 1024 } = {}) {
  await mkdir(destination, { recursive: true, mode: 0o700 })
  let bytes = 0
  let entries = 0
  const check = (name, size, type) => {
    const relative = name.replace(/\/$/, '')
    safeRelativePath(relative)
    if (asset.root && relative !== asset.root && !relative.startsWith(asset.root + '/') &&
        !(asset.platform === 'darwin' && (relative === '__MACOSX' || relative.startsWith('__MACOSX/')))) {
      throw new Error('Archive contains a path outside the expected application root')
    }
    if (!['File', 'Directory', 'OldFile'].includes(type)) throw new Error('Archive links and special file types are not allowed')
    if (!Number.isSafeInteger(size) || size < 0 || ++entries > 10_000 || (bytes += size) > maxBytes) {
      throw new Error('Archive extraction exceeds its size or entry limit')
    }
    return path.join(destination, ...relative.split('/'))
  }
  if (asset.format === 'tar') {
    await tar.x({
      file, cwd: destination, strict: true, preserveOwner: false,
      filter(name, entry) {
        try {
          check(name, entry.size, entry.type)
          return true
        } catch (error) {
          this.abort(error)
          return false
        }
      },
    })
    return
  }
  if (asset.format !== 'zip') throw new Error(`Unsupported archive format: ${asset.format}`)
  await new Promise((resolve, reject) => {
    yauzl.open(file, { lazyEntries: true, autoClose: false, strictFileNames: true, validateEntrySizes: true }, (error, zip) => {
      if (error) { reject(error); return }
      const fail = (cause) => { zip.close(); reject(cause) }
      zip.once('error', fail)
      zip.once('end', () => { zip.close(); resolve() })
      zip.on('entry', (entry) => {
        const extract = async () => {
          const mode = entry.externalFileAttributes >>> 16
          const fileType = mode & 0o170000
          if (fileType && ![0o100000, 0o040000].includes(fileType)) throw new Error('ZIP archive links and special files are not allowed')
          const directory = entry.fileName.endsWith('/')
          const target = check(entry.fileName, entry.uncompressedSize, directory ? 'Directory' : 'File')
          if (directory) {
            await mkdir(target, { recursive: true, mode: 0o755 })
            return
          }
          await mkdir(path.dirname(target), { recursive: true, mode: 0o755 })
          const stream = await new Promise((accept, decline) => {
            zip.openReadStream(entry, (err, value) => err ? decline(err) : accept(value))
          })
          await pipeline(stream, createWriteStream(target, { flags: 'wx', mode: (mode & 0o777) || 0o644 }))
        }
        extract().then(() => zip.readEntry(), fail)
      })
      zip.readEntry()
    })
  })
}
