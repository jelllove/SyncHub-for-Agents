import { createHash } from 'node:crypto'
import { createReadStream } from 'node:fs'
import { chmod, lstat, mkdir, mkdtemp, open, readFile, rename, rm, writeFile } from 'node:fs/promises'
import path from 'node:path'
import release from '../release.json' with { type: 'json' }
import { selectPlatform } from './platform.mjs'
import { downloadArchive } from './download.mjs'
import { extractArchive } from './extract.mjs'

export const defaultPackageRoot = path.resolve(import.meta.dirname, '..')

async function fileDigest(file) {
  const hash = createHash('sha256')
  for await (const bytes of createReadStream(file)) hash.update(bytes)
  return hash.digest('hex')
}

async function existingInstallation(native, asset) {
  let directory
  try { directory = await lstat(native) } catch (error) {
    if (error.code === 'ENOENT') return null
    throw error
  }
  if (!directory.isDirectory() || directory.isSymbolicLink()) throw new Error('Native installation directory is invalid; reinstall the NPM package')
  let receipt
  try {
    const marker = path.join(native, 'install.json')
    const markerStat = await lstat(marker)
    if (!markerStat.isFile() || markerStat.isSymbolicLink()) throw new Error('Integrity receipt must be a regular file')
    receipt = JSON.parse(await readFile(marker, 'utf8'))
  } catch (cause) {
    throw new Error('Native integrity receipt is missing or unreadable; reinstall the NPM package', { cause })
  }
  if (typeof receipt !== 'object' || receipt === null || receipt.version !== asset.version || receipt.platform !== asset.platform ||
      receipt.arch !== asset.arch || receipt.archiveSHA256 !== asset.sha256 ||
      !/^[a-f0-9]{64}$/.test(receipt.executableSHA256)) {
    throw new Error('Native integrity receipt does not match this NPM installation')
  }
  const executable = path.join(native, ...asset.executable.split('/'))
  let parent = native
  for (const part of asset.executable.split('/').slice(0, -1)) {
    parent = path.join(parent, part)
    const directory = await lstat(parent)
    if (!directory.isDirectory() || directory.isSymbolicLink()) throw new Error('Native executable parent path failed integrity validation')
  }
  const stat = await lstat(executable)
  if (!stat.isFile() || stat.isSymbolicLink() || await fileDigest(executable) !== receipt.executableSHA256) {
    throw new Error('Installed desktop executable failed integrity validation or was modified; reinstall the NPM package')
  }
  return { executable, version: asset.version }
}

export async function ensureInstalled({
  packageRoot = defaultPackageRoot, manifest = release,
  platform = process.platform, arch = process.arch, fetchImpl = fetch,
} = {}) {
  const asset = selectPlatform(manifest, platform, arch)
  const native = path.join(packageRoot, 'native')
  const existing = await existingInstallation(native, asset)
  if (existing) return existing
  await mkdir(packageRoot, { recursive: true })
  const lockPath = path.join(packageRoot, '.install-lock')
  let lock
  try { lock = await open(lockPath, 'wx', 0o600) } catch (cause) {
    throw new Error('Native installation is locked or not writable; wait for another install or reinstall the NPM package', { cause })
  }
  let staging
  try {
    const raced = await existingInstallation(native, asset)
    if (raced) return raced
    staging = await mkdtemp(path.join(packageRoot, '.install-stage-'))
    const archive = path.join(staging, asset.name)
    const extracted = path.join(staging, 'extracted')
    await downloadArchive(asset, archive, { fetchImpl })
    await extractArchive(archive, extracted, asset)
    const payload = path.join(extracted, ...asset.root.split('/').filter(Boolean))
    const executable = path.join(payload, ...asset.executable.split('/'))
    const stat = await lstat(executable)
    if (!stat.isFile() || stat.isSymbolicLink() || stat.size === 0) throw new Error('Verified archive has no valid desktop executable')
    if (platform !== 'win32') await chmod(executable, 0o755)
    await writeFile(path.join(payload, 'install.json'), JSON.stringify({
      version: asset.version, platform, arch, archiveSHA256: asset.sha256,
      executableSHA256: await fileDigest(executable),
    }) + '\n', { flag: 'wx', mode: 0o600 })
    await rename(payload, native)
    return { executable: path.join(native, ...asset.executable.split('/')), version: asset.version }
  } finally {
    if (staging) await rm(staging, { recursive: true, force: true })
    await lock.close()
    await rm(lockPath, { force: true })
  }
}
