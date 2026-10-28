import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { exportScreenshots, validateTag, verifyEvidence } from './macos-evidence.mjs'
import { createHash } from 'node:crypto'
import { cpSync } from 'node:fs'
import { publishMacOS } from './macos-publish.mjs'

test('macOS release tags reject shell syntax, malformed versions and leading zeros', () => {
  for (const tag of ['v0.3.4', 'v1.2.3-rc.1']) {
    assert.equal(validateTag(tag), tag.slice(1))
  }
  for (const tag of ['', 'main', 'v01.2.3', 'v1.2', 'v1.2.3;echo hi', 'v1.2.3/../../x', 'v1.2.3-01']) {
    assert.throws(() => validateTag(tag), /release tag/)
  }
})

function fixture() {
  const directory = mkdtempSync(join(tmpdir(), 'synchub-macos-evidence-'))
  const screenshots = join(directory, 'screenshots')
  mkdirSync(screenshots)
  const png = Buffer.alloc(4096)
  Buffer.from('89504e470d0a1a0a', 'hex').copy(png)
  png.writeUInt32BE(13, 8)
  png.write('IHDR', 12)
  png.writeUInt32BE(1100, 16)
  png.writeUInt32BE(760, 20)
  for (const name of ['01-welcome', '02-repository', '03-validation']) {
    writeFileSync(join(screenshots, `${name}.png`), png)
  }
  const summary = { result: 'Passed', passedTests: 1, failedTests: 0, skippedTests: 0 }
  const receipt = {
    version: '0.3.4', architecture: 'arm64', installedFromDMG: true,
    adHocSignatureVerified: true, executableVersionVerified: true,
    isolatedHome: true, originalDMGDigest: 'a'.repeat(64),
  }
  return { directory, screenshots, summary, receipt }
}

test('publication evidence requires real native test success, installation receipt and every screenshot', () => {
  const f = fixture()
  try {
    assert.doesNotThrow(() => verifyEvidence(f.directory, f.summary, f.receipt, 'v0.3.4', 'arm64'))
    for (const bad of [
      { ...f.summary, result: 'Failed' }, { ...f.summary, failedTests: 1 },
      { ...f.summary, skippedTests: 1 }, { ...f.summary, passedTests: 0 },
    ]) {
      assert.throws(() => verifyEvidence(f.directory, bad, f.receipt, 'v0.3.4', 'arm64'), /native tests/)
    }
    for (const field of ['installedFromDMG', 'adHocSignatureVerified', 'executableVersionVerified', 'isolatedHome']) {
      assert.throws(
        () => verifyEvidence(f.directory, f.summary, { ...f.receipt, [field]: false }, 'v0.3.4', 'arm64'),
        /installation receipt/,
      )
    }
    assert.throws(() => verifyEvidence(f.directory, f.summary, f.receipt, 'v0.3.5', 'arm64'), /installation receipt/)
    assert.throws(() => verifyEvidence(f.directory, f.summary, f.receipt, 'v0.3.4', 'x86_64'), /installation receipt/)
    rmSync(join(f.screenshots, '02-repository.png'))
    assert.throws(() => verifyEvidence(f.directory, f.summary, f.receipt, 'v0.3.4', 'arm64'), /02-repository/)
  } finally {
    rmSync(f.directory, { recursive: true, force: true })
  }
})

test('blank, truncated, or non-PNG captures do not qualify as screenshot evidence', () => {
  const f = fixture()
  try {
    for (const image of [Buffer.alloc(0), Buffer.alloc(4096), Buffer.from('not a screenshot')]) {
      writeFileSync(join(f.screenshots, '01-welcome.png'), image)
      assert.throws(() => verifyEvidence(f.directory, f.summary, f.receipt, 'v0.3.4', 'arm64'), /screenshot/)
    }
  } finally {
    rmSync(f.directory, { recursive: true, force: true })
  }
})

test('publication resolves live lightweight and annotated tags before any remote mutation', () => {
  const f = fixture()
  try {
    const commit = 'b'.repeat(40)
    const dmg = Buffer.from('synthetic package')
    const digest = createHash('sha256').update(dmg).digest('hex')
    writeFileSync(join(f.directory, 'SyncHub-macOS-universal-adhoc.dmg'), dmg)
    writeFileSync(join(f.directory, 'SHA256SUMS-macOS.txt'), `${digest}  SyncHub-macOS-universal-adhoc.dmg\n`)
    writeFileSync(join(f.directory, 'build-receipt.json'), JSON.stringify({
      tag: 'v0.3.4', sourceCommit: commit, dmgSHA256: digest, signature: 'ad-hoc', notarized: false,
    }))
    for (const architecture of ['arm64', 'x86_64']) {
      const root = join(f.directory, `macos-native-${architecture}`)
      mkdirSync(root)
      cpSync(f.screenshots, join(root, 'screenshots'), { recursive: true })
      writeFileSync(join(root, 'test-summary.json'), JSON.stringify(f.summary))
      writeFileSync(join(root, 'installation.json'), JSON.stringify({
        ...f.receipt, architecture, originalDMGDigest: digest,
      }))
    }
    for (const target of ['matching', 'annotated', 'moved', 'missing', 'tree', 'cycle']) {
      const mutations = []
      const gh = (args) => {
        if (args[0] === 'api') {
          if (target === 'missing') throw new Error('GitHub tag lookup failed')
          const annotated = target === 'annotated' && args[1].includes('/git/ref/')
          const type = target === 'tree' ? 'tree' : (annotated || target === 'cycle' ? 'tag' : 'commit')
          const sha = target === 'moved' ? 'c'.repeat(40) : commit
          return JSON.stringify({ object: { type, sha } })
        }
        if (args[0] === 'release' && args[1] === 'view') {
          return JSON.stringify({ body: 'Existing notes', isDraft: false, assets: [] })
        }
        mutations.push(args)
        return ''
      }
      const publish = () => publishMacOS(f.directory, f.directory, 'v0.3.4', commit,
        'https://github.com/jelllove/SyncHub-for-Agents/actions/runs/123', gh)
      if (target === 'matching' || target === 'annotated') {
        assert.doesNotThrow(publish)
        assert.deepEqual(mutations.map((args) => args[1]), ['upload', 'edit'])
      } else {
        assert.throws(publish, /tag|commit/)
        assert.equal(mutations.length, 0, `${target} tag must prevent uploads and notes editing`)
      }
    }
  } finally {
    rmSync(f.directory, { recursive: true, force: true })
  }
})

test('native screenshots are extracted from XCTest attachments without runner filesystem access', () => {
  const f = fixture()
  try {
    const attachments = join(f.screenshots, 'attachments')
    mkdirSync(attachments)
    const items = []
    for (const name of ['01-welcome', '02-repository', '03-validation']) {
      const file = `${name}-export.png`
      cpSync(join(f.screenshots, `${name}.png`), join(attachments, file))
      rmSync(join(f.screenshots, `${name}.png`))
      items.push({ exportedFileName: file, suggestedHumanReadableName: `${name}_1.png` })
    }
    const manifest = join(attachments, 'manifest.json')
    writeFileSync(manifest, JSON.stringify([{ testName: 'NativeSmokeTests', attachments: items }]))
    assert.doesNotThrow(() => exportScreenshots(f.directory))
    assert.doesNotThrow(() => verifyEvidence(f.directory, f.summary, f.receipt, 'v0.3.4', 'arm64'))
    writeFileSync(manifest, JSON.stringify([{ attachments: [items[0]] }]))
    assert.throws(() => exportScreenshots(f.directory), /02-repository/)
    writeFileSync(manifest, JSON.stringify([{ attachments: [{ ...items[0], exportedFileName: '../outside.png' }] }]))
    assert.throws(() => exportScreenshots(f.directory), /attachment/)
  } finally {
    rmSync(f.directory, { recursive: true, force: true })
  }
})
