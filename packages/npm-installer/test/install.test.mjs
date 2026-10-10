import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import test from 'node:test'
import * as tar from 'tar'
import release from '../release.json' with { type: 'json' }
import { extractArchive } from '../lib/extract.mjs'
import { ensureInstalled } from '../lib/install.mjs'
import { tarFixture, zipFixture } from './fixtures.mjs'

test('tar archives reject traversal and link types', async () => {
  const root = await mkdtemp(path.join(tmpdir(), 'synchub-npm-tar-reject-'))
  try {
    for (const bytes of [tarFixture('../escape'), tarFixture('SyncHub/link', '2', '../../outside')]) {
      const archive = path.join(root, 'unsafe.tar.gz')
      await writeFile(archive, bytes)
      await assert.rejects(extractArchive(archive, path.join(root, 'extract'), {
        format: 'tar', root: 'SyncHub', executable: 'SyncHub',
      }), /path|link|special|outside/i)
    }
  } finally {
    await rm(root, { recursive: true, force: true })
  }
})

test('ZIP extraction rejects traversal, symlinks and oversized entries before publication', async () => {
  const root = await mkdtemp(path.join(tmpdir(), 'synchub-npm-zip-'))
  try {
    for (const entry of [
      { name: '../escape', body: 'bad' },
      { name: '/absolute', body: 'bad' },
      { name: 'SyncHub.exe', body: '../outside', mode: 0o120777 },
      { name: 'SyncHub.exe', body: 'oversized-content' },
    ]) {
      const archive = path.join(root, 'unsafe.zip')
      await writeFile(archive, zipFixture([entry]))
      await assert.rejects(extractArchive(archive, path.join(root, 'extract'), {
        format: 'zip', root: '', executable: 'SyncHub.exe',
      }, { maxBytes: 4 }), /path|link|size|large|absolute|invalid/i)
    }
  } finally {
    await rm(root, { recursive: true, force: true })
  }
})

test('native installation is verified, idempotent and rejects a tampered executable', async () => {
  const root = await mkdtemp(path.join(tmpdir(), 'synchub-npm-install-'))
  const bytes = zipFixture([{ name: 'SyncHub.exe', body: 'synthetic executable' }])
  const manifest = {
    ...release, assets: {
      ...release.assets, windows: {
        ...release.assets.windows, name: 'fixture.zip', size: bytes.length,
        sha256: createHash('sha256').update(bytes).digest('hex'),
      },
    },
  }
  let requests = 0
  const options = {
    packageRoot: root, manifest, platform: 'win32', arch: 'x64',
    fetchImpl: async () => { requests++; return new Response(bytes) },
  }
  try {
    const installed = await ensureInstalled(options)
    assert.equal(await readFile(installed.executable, 'utf8'), 'synthetic executable')
    const again = await ensureInstalled(options)
    assert.equal(again.executable, installed.executable)
    assert.equal(requests, 1)
    await writeFile(installed.executable, 'tampered executable')
    await assert.rejects(ensureInstalled(options), /integrity|modified|checksum/i)
  } finally {
    await rm(root, { recursive: true, force: true })
  }
})

test('failed verification leaves neither a native installation nor a lock', async () => {
  const root = await mkdtemp(path.join(tmpdir(), 'synchub-npm-partial-'))
  try {
    await assert.rejects(ensureInstalled({
      packageRoot: root, manifest: release, platform: 'win32', arch: 'x64',
      fetchImpl: async () => new Response(Buffer.from('wrong bytes')),
    }), /size|SHA-256/)
    await assert.rejects(readFile(path.join(root, 'native', 'install.json')), /ENOENT/)
    await assert.rejects(readFile(path.join(root, '.install-lock')), /ENOENT/)
  } finally {
    await rm(root, { recursive: true, force: true })
  }
})

test('Linux tar extraction preserves an executable inside the expected root', async () => {
  const root = await mkdtemp(path.join(tmpdir(), 'synchub-npm-tar-'))
  try {
    const source = path.join(root, 'source')
    await mkdir(path.join(source, 'SyncHub'), { recursive: true })
    await writeFile(path.join(source, 'SyncHub', 'SyncHub'), 'synthetic Linux binary', { mode: 0o755 })
    const archive = path.join(root, 'fixture.tar.gz')
    await tar.c({ cwd: source, file: archive, gzip: true }, ['SyncHub'])
    const target = path.join(root, 'extract')
    await extractArchive(archive, target, { format: 'tar', root: 'SyncHub', executable: 'SyncHub' })
    assert.equal(await readFile(path.join(target, 'SyncHub', 'SyncHub'), 'utf8'), 'synthetic Linux binary')
  } finally {
    await rm(root, { recursive: true, force: true })
  }
})
