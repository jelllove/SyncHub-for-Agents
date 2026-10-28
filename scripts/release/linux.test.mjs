import assert from 'node:assert/strict'
import { createHash } from 'node:crypto'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import test from 'node:test'

const root = new URL('../../', import.meta.url)
const read = (name) => readFileSync(new URL(name, root), 'utf8').replace(/\r\n/g, '\n')
const section = (text, name) => text.split(`\n  ${name}:\n`)[1]?.split(/\n  [^ \n]/)[0] ?? ''

test('default Linux packaging includes AppImage, DEB and RPM with matching version and architecture', () => {
  const tasks = read('build/linux/Taskfile.yml')
  const packages = section(tasks, 'package')
  for (const format of ['appimage', 'deb', 'rpm']) {
    assert.match(packages, new RegExp(`task: create:${format}\\b`))
  }
  for (const format of ['deb', 'rpm']) {
    const task = section(tasks, `generate:${format}`)
    assert.match(task, /VERSION: '\{\{\.VERSION \| default "0\.1\.0"\}\}'/)
    assert.match(task, /GOARCH: '\{\{\.ARCH \| default ARCH\}\}'/)
  }
})

test('CI and releases retain RPM artifacts and native validation checks RPM metadata and launch', () => {
  for (const workflow of ['ci.yml', 'release.yml']) {
    assert.match(read(`.github/workflows/${workflow}`), /bin\/\*\.rpm/)
  }
  const workflow = read('.github/workflows/linux-installer.yml')
  assert.match(workflow, /rpm -qp/)
  assert.match(workflow, /bsdtar --extract --file "\$rpm_package" --directory "\$evidence\/rpm" --no-same-owner/)
  assert.doesNotMatch(workflow, /rpm2cpio/)
  assert.match(workflow, /apt-get install --yes .*libarchive-tools/)
  assert.match(read('.github/workflows/ci.yml'), /apt-get install --yes .*libarchive-tools/)
  assert.match(workflow, /for format in appimage deb rpm/)
  assert.match(workflow, /rpmLaunchPassed:true/)
  assert.match(workflow, /linux-rpm-install\.sh/)
  assert.match(workflow, /node "\$GITHUB_WORKSPACE\/tooling\/scripts\/release\/linux-evidence\.mjs" version "\$RELEASE_TAG" rpm/)
  const smoke = read('scripts/smoke/linux.sh')
  assert.match(smoke, /bsdtar --extract --file "\$rpm_package" --directory "\$work_dir\/rpm" --no-same-owner/)
  assert.doesNotMatch(smoke, /rpm2cpio/)
  assert.match(smoke, /rpm_version="\$\("\$work_dir\/rpm\/usr\/bin\/SyncHub" --version\)"/)
  assert.match(smoke, /deb_version="\$\("\$work_dir\/deb\/usr\/bin\/SyncHub" --version\)"/)
  assert.match(smoke, /test -n "\$rpm_version"/)
  assert.match(smoke, /test "\$rpm_version" = "\$deb_version"/)
  const install = read('scripts/smoke/linux-rpm-install.sh')
  assert.match(install, /dnf install/)
  assert.match(install, /fedora:43/)
})

test('published Linux asset detection accepts none, legacy DEB set, or complete DEB/RPM set only', async () => {
  const { publishedPackageMode } = await import('./linux-evidence.mjs')
  const legacy = ['SyncHub-x86_64.AppImage', 'SyncHub.deb', 'SHA256SUMS-Linux.txt']
  const assets = (names) => names.map((name) => ({ name }))
  assert.equal(publishedPackageMode(assets(['windows.exe'])), 'build')
  assert.equal(publishedPackageMode(assets(legacy)), 'legacy')
  assert.equal(publishedPackageMode(assets([...legacy, 'SyncHub.rpm'])), 'complete')
  for (const names of [['SyncHub.rpm'], legacy.slice(0, 2), [...legacy, 'SyncHub.rpm', 'SyncHub.rpm']]) {
    assert.throws(() => publishedPackageMode(assets(names)), /Linux release asset set/)
  }
})

test('Linux checksum verification rejects missing RPM, duplicate names, traversal and wrong bytes', async () => {
  const { verifyPackageManifest } = await import('./linux-evidence.mjs')
  const directory = mkdtempSync(join(tmpdir(), 'synchub-linux-packages-'))
  try {
    const names = ['SyncHub-x86_64.AppImage', 'SyncHub.deb', 'SyncHub.rpm']
    const lines = names.map((name) => {
      const bytes = Buffer.from(`synthetic ${name}`)
      writeFileSync(join(directory, name), bytes)
      return `${createHash('sha256').update(bytes).digest('hex')}  ${name}`
    })

    const manifest = join(directory, 'SHA256SUMS-Linux.txt')
    const write = (entries) => writeFileSync(manifest, entries.join('\n') + '\n')
    write(lines)
    assert.doesNotThrow(() => verifyPackageManifest(directory, 'complete'))
    write(lines.slice(0, 2))
    assert.doesNotThrow(() => verifyPackageManifest(directory, 'legacy'))
    assert.throws(() => verifyPackageManifest(directory, 'complete'), /package set/)
    for (const entries of [
      [...lines, lines[0]],
      [lines[0].replace('SyncHub-x86_64.AppImage', '../private'), ...lines.slice(1)],
      [lines[0].replace(/^[a-f0-9]{64}/, '0'.repeat(64)), ...lines.slice(1)],
    ]) {
      write(entries)
      assert.throws(() => verifyPackageManifest(directory, 'complete'), /checksum|package set/)
    }
    assert.throws(() => verifyPackageManifest(directory, 'unexpected'), /mode/)
  } finally {
    rmSync(directory, { recursive: true, force: true })
  }
})

test('Linux package metadata versions follow nFPM stable and prerelease ordering', async () => {
  const { packageVersion } = await import('./linux-evidence.mjs')
  for (const format of ['deb', 'rpm']) {
    assert.equal(packageVersion('v0.3.4', format), '0.3.4-1')
    assert.equal(packageVersion('v1.2.3-rc.1', format), '1.2.3~rc.1-1')
  }
  assert.equal(packageVersion('v1.2.3-alpha-beta.1', 'deb'), '1.2.3~alpha-beta.1-1')
  assert.equal(packageVersion('v1.2.3-alpha-beta.1', 'rpm'), '1.2.3~alpha_beta.1-1')
  assert.throws(() => packageVersion('main', 'rpm'), /release tag/)
  assert.throws(() => packageVersion('v0.3.4', 'unknown'), /format/)
})
