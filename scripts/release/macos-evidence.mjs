import { copyFileSync, readFileSync } from 'node:fs'
import { join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

export function validateTag(tag) {
  const number = '(?:0|[1-9][0-9]*)'
  const identifier = '(?:0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)'
  const pattern = new RegExp(`^v${number}\\.${number}\\.${number}(?:-${identifier}(?:\\.${identifier})*)?$`)
  if (typeof tag !== 'string' || !pattern.test(tag)) {
    throw new Error('Expected a semantic release tag such as v0.3.4 or v0.3.5-rc.1')
  }
  return tag.slice(1)
}

export function verifyEvidence(directory, summary, receipt, tag, architecture) {
  const version = validateTag(tag)
  if (summary.result !== 'Passed' || !Number.isInteger(summary.passedTests) || summary.passedTests < 1 ||
      summary.failedTests !== 0 || summary.skippedTests !== 0) {
    throw new Error('macOS native tests did not all pass without skips')
  }
  if (receipt.version !== version || receipt.architecture !== architecture ||
      !['arm64', 'x86_64'].includes(architecture) ||
      !/^[a-f0-9]{64}$/.test(receipt.originalDMGDigest ?? '') ||
      ['installedFromDMG', 'adHocSignatureVerified', 'executableVersionVerified', 'isolatedHome']
        .some((field) => receipt[field] !== true)) {
    throw new Error('macOS installation receipt is missing or mismatched')
  }
  for (const name of ['01-welcome', '02-repository', '03-validation']) {
    const image = readFileSync(join(directory, 'screenshots', `${name}.png`))
    if (image.length < 1024 || !image.subarray(0, 8).equals(Buffer.from('89504e470d0a1a0a', 'hex')) ||
        image.toString('ascii', 12, 16) !== 'IHDR' ||
        image.readUInt32BE(16) < 300 || image.readUInt32BE(20) < 200) {
      throw new Error(`Invalid native screenshot: ${name}`)
    }
  }
}

export function exportScreenshots(directory) {
  const root = join(directory, 'screenshots')
  const attachments = join(root, 'attachments')
  const manifest = JSON.parse(readFileSync(join(attachments, 'manifest.json'), 'utf8'))
  const captures = []
  function collect(value) {
    if (!value || typeof value !== 'object') return
    if (typeof value.exportedFileName === 'string') captures.push(value)
    for (const child of Object.values(value)) collect(child)
  }
  collect(manifest)
  for (const name of ['01-welcome', '02-repository', '03-validation']) {
    const matches = captures.filter((capture) =>
      typeof capture.suggestedHumanReadableName === 'string' &&
      new RegExp(`^${name}(?:[._ -]|$)`).test(capture.suggestedHumanReadableName))
    if (matches.length !== 1) throw new Error(`Expected one XCTest screenshot attachment: ${name}`)
    const file = matches[0].exportedFileName
    if (/[/\\]/.test(file) || !file.endsWith('.png')) throw new Error('Invalid exported screenshot attachment path')
    copyFileSync(join(attachments, file), join(root, `${name}.png`))
  }
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const [command, ...args] = process.argv.slice(2)
  if (command === 'version' && args.length === 1) {
    console.log(validateTag(args[0]))
  } else if (command === 'screenshots' && args.length === 1) {
    exportScreenshots(args[0])
  } else if (command === 'verify' && args.length === 3) {
    const [directory, tag, architecture] = args
    verifyEvidence(
      directory,
      JSON.parse(readFileSync(join(directory, 'test-summary.json'), 'utf8')),
      JSON.parse(readFileSync(join(directory, 'installation.json'), 'utf8')),
      tag, architecture,
    )
    console.log(`Native installed-app evidence verified: ${tag} ${architecture}`)
  } else {
    throw new Error('Usage: macos-evidence.mjs version TAG | screenshots DIRECTORY | verify DIRECTORY TAG ARCHITECTURE')
  }
}
