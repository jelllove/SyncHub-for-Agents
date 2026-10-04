import { execFileSync } from 'node:child_process'
import { createHash } from 'node:crypto'
import { readFileSync, writeFileSync } from 'node:fs'
import { join } from 'node:path'
import { validateTag, verifyEvidence } from './macos-evidence.mjs'

const [directory, evidence, tag, expectedCommit, runURL] = process.argv.slice(2)
validateTag(tag)
if (!/^[a-f0-9]{40}$/.test(expectedCommit ?? '') || !/^https:\/\/github\.com\/[^/]+\/[^/]+\/actions\/runs\/[0-9]+$/.test(runURL ?? '')) {
  throw new Error('Publication requires an immutable source commit and hosted validation URL')
}
const readJSON = (filename) => JSON.parse(readFileSync(filename, 'utf8'))
const receipt = readJSON(join(directory, 'build-receipt.json'))
const filename = 'SyncHub-macOS-universal-adhoc.dmg'
const digest = createHash('sha256').update(readFileSync(join(directory, filename))).digest('hex')
if (receipt.tag !== tag || receipt.sourceCommit !== expectedCommit || receipt.dmgSHA256 !== digest ||
    receipt.signature !== 'ad-hoc' || receipt.notarized !== false) {
  throw new Error('Build receipt does not match the exact release package')
}
for (const architecture of ['arm64', 'x86_64']) {
  const root = join(evidence, `macos-native-${architecture}`)
  const installation = readJSON(join(root, 'installation.json'))
  verifyEvidence(root, readJSON(join(root, 'test-summary.json')), installation, tag, architecture)
  if (installation.originalDMGDigest !== digest) {
    throw new Error('Native tests did not install this exact DMG')
  }
}
const checksums = readFileSync(join(directory, 'SHA256SUMS-macOS.txt'), 'utf8')
if (checksums.trim() !== `${digest}  ${filename}`) {
  throw new Error('macOS checksum manifest differs from tested DMG')
}
const gh = (args) => execFileSync('gh', args, { encoding: 'utf8', stdio: ['ignore', 'pipe', 'inherit'] })
const release = readJSONFromGitHub()
function readJSONFromGitHub() {
  return JSON.parse(gh(['release', 'view', tag, '--json', 'body,isDraft,isPrerelease,assets']))
}
if (release.isDraft) {
  throw new Error('macOS asset publication requires an already published release')
}
for (const name of [filename, 'SHA256SUMS-macOS.txt']) {
  if (release.assets.some((asset) => asset.name === name)) {
    throw new Error(`Refusing to replace existing release asset: ${name}`)
  }
}
gh(['release', 'upload', tag, join(directory, filename), join(directory, 'SHA256SUMS-macOS.txt')])
const notesFile = join(directory, 'macos-release-notes.txt')
const section = `\n\n## macOS ad-hoc test build\n\n` +
  `Download \`${filename}\` (Apple Silicon and Intel); verify it with \`SHA256SUMS-macOS.txt\`.\n\n` +
  `**Ad-hoc signed, NOT Apple Developer ID signed and NOT notarized.** ` +
  `Gatekeeper may block first launch. Only allow the app if you trust its source; do not disable system-wide security.\n\n` +
  `The exact DMG passed mount/copy installation, executable/version/signature checks and native installed-app ` +
  `XCUITest onboarding/invalid-input tests on Apple Silicon and Intel. ` +
  `[Screenshots, xcresult and logs](${runURL}) are workflow artifacts.\n\n` +
  `CI does not establish Gatekeeper acceptance, keychain/login-item behavior, live sync, or compatibility with every macOS version. ` +
  `Minimum build target is macOS 12; these native tests ran on macOS 15.\n`
writeFileSync(notesFile, release.body + section)
gh(['release', 'edit', tag, '--notes-file', notesFile])
console.log(`Uploaded verified macOS ad-hoc installer to ${tag}`)
