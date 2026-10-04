import assert from 'node:assert/strict'
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'
import { validateTag, verifyEvidence } from './macos-evidence.mjs'

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
