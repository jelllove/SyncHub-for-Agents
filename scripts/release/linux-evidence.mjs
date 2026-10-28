import { createHash } from 'node:crypto'
import { readFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { validateTag } from './macos-evidence.mjs'

const legacyNames = ['SyncHub-x86_64.AppImage', 'SyncHub.deb']
const packageNames = [...legacyNames, 'SyncHub.rpm']
const manifestName = 'SHA256SUMS-Linux.txt'

export function packageVersion(tag, format) {
  if (!['deb', 'rpm'].includes(format)) throw new Error('Invalid Linux package format')
  const version = validateTag(tag)
  const separator = version.indexOf('-')
  if (separator === -1) return `${version}-1`
  const prerelease = version.slice(separator + 1)
  return `${version.slice(0, separator)}~${format === 'rpm' ? prerelease.replaceAll('-', '_') : prerelease}-1`
}

export function publishedPackageMode(assets) {
  const relevant = assets.filter((asset) => [...packageNames, manifestName].includes(asset.name))
  const names = relevant.map((asset) => asset.name)
  if (names.length === 0) return 'build'
  if (new Set(names).size !== names.length) throw new Error('Duplicate Linux release asset set')
  if (names.length === 3 && [...legacyNames, manifestName].every((name) => names.includes(name))) return 'legacy'
  if (names.length === 4 && [...packageNames, manifestName].every((name) => names.includes(name))) return 'complete'
  throw new Error('Incomplete Linux release asset set')
}

export function verifyPackageManifest(directory, mode) {
  if (!['legacy', 'complete'].includes(mode)) throw new Error('Invalid Linux package verification mode')
  const expected = mode === 'legacy' ? legacyNames : packageNames
  const lines = readFileSync(join(directory, manifestName), 'utf8').trim().split(/\r?\n/)
  const found = new Set()
  for (const line of lines) {
    const match = /^([a-f0-9]{64})  ([A-Za-z0-9._-]+)$/.exec(line)
    if (!match || !expected.includes(match[2]) || found.has(match[2])) throw new Error('Invalid Linux checksum package set')
    const [, digest, name] = match
    found.add(name)
    const actual = createHash('sha256').update(readFileSync(join(directory, name))).digest('hex')
    if (actual !== digest) throw new Error(`Linux package checksum mismatch: ${name}`)
  }
  if (found.size !== expected.length) throw new Error('Incomplete Linux checksum package set')
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [command, argument, mode] = process.argv.slice(2)
  if (command === 'published-mode' && argument && !mode) {
    console.log(publishedPackageMode(JSON.parse(readFileSync(argument, 'utf8')).assets))
  } else if (command === 'verify' && argument && mode) {
    verifyPackageManifest(argument, mode)
    console.log(`Linux package checksums verified: ${mode}`)
  } else if (command === 'version' && argument && mode) {
    console.log(packageVersion(argument, mode))
  } else {
    throw new Error('Usage: linux-evidence.mjs published-mode RELEASE_JSON | verify DIRECTORY MODE | version TAG deb|rpm')
  }
}
