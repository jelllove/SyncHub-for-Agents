import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { mkdtemp, readFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import test from 'node:test'
import release from '../release.json' with { type: 'json' }
import { selectPlatform } from '../lib/platform.mjs'
import { downloadArchive } from '../lib/download.mjs'

test('platform selection is explicit and macOS uses one universal archive', () => {
  assert.equal(selectPlatform(release, 'win32', 'x64').executable, 'SyncHub.exe')
  assert.equal(selectPlatform(release, 'darwin', 'x64').name, selectPlatform(release, 'darwin', 'arm64').name)
  assert.equal(selectPlatform(release, 'linux', 'x64').format, 'tar')
  for (const [platform, arch] of [['win32', 'arm64'], ['linux', 'arm64'], ['freebsd', 'x64']]) {
    assert.throws(() => selectPlatform(release, platform, arch), /unsupported/i)
  }
  assert.throws(() => selectPlatform({ ...release, repository: 'https://example.com' }, 'win32', 'x64'), /repository/i)
})

test('download verifies exact size and hash without credentials and rejects unsafe redirects', async () => {
  const root = await mkdtemp(path.join(tmpdir(), 'synchub-npm-download-'))
  const bytes = Buffer.from('synthetic archive')
  const asset = {
    url: 'https://github.com/jelllove/SyncHub-for-Agents/releases/download/v0.3.5/fixture.zip',
    size: bytes.length, sha256: createHash('sha256').update(bytes).digest('hex'),
  }
  try {
    const file = path.join(root, 'archive.zip')
    let requests = 0
    const fetchImpl = async (_url, options) => {
      requests++
      assert.equal(options.redirect, 'manual')
      assert.equal(options.headers.Authorization, undefined)
      if (requests === 1) throw new Error('fetch failed')
      return new Response(bytes, { status: 200 })
    }
    await downloadArchive(asset, file, { fetchImpl, retryDelay: 0 })
    assert.equal(requests, 2)
    assert.deepEqual(await readFile(file), bytes)
    await assert.rejects(downloadArchive({ ...asset, sha256: '0'.repeat(64) }, file,
      { fetchImpl: async () => new Response(bytes), retryDelay: 0 }), /SHA-256/)
    await assert.rejects(downloadArchive({ ...asset, size: bytes.length - 1 }, file,
      { fetchImpl: async () => new Response(bytes), retryDelay: 0 }), /size/i)
    await assert.rejects(downloadArchive(asset, file,
      { fetchImpl: async () => new Response(null, { status: 302, headers: { location: 'http://example.com/evil' } }) }), /HTTPS|host/i)
  } finally {
    await rm(root, { recursive: true, force: true })
  }
})

test('persistent transport and permanent HTTP failures do not become successful installation', async () => {
  const root = await mkdtemp(path.join(tmpdir(), 'synchub-npm-fail-'))
  const asset = {
    url: 'https://github.com/jelllove/SyncHub-for-Agents/releases/download/v0.3.5/fixture.zip',
    size: 1, sha256: '0'.repeat(64),
  }
  try {
    let calls = 0
    await assert.rejects(downloadArchive(asset, path.join(root, 'fixture.zip'), {
      retryDelay: 0, fetchImpl: async () => { calls++; throw new Error('fetch failed') },
    }), /3 attempts/)
    assert.equal(calls, 3)
    calls = 0
    await assert.rejects(downloadArchive(asset, path.join(root, 'fixture.zip'), {
      retryDelay: 0, fetchImpl: async () => { calls++; return new Response(null, { status: 403 }) },
    }), /403/)
    assert.equal(calls, 1)
  } finally {
    await rm(root, { recursive: true, force: true })
  }
})
