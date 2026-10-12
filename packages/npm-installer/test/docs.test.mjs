import assert from 'node:assert/strict'
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import path from 'node:path'
import test from 'node:test'
import { checkDocs, requirements } from '../scripts/check-docs.mjs'

const root = path.resolve(import.meta.dirname, '..')

test('real package delivery documents are complete and linked', async () => {
  await checkDocs(root)
})

test('documentation gates reject missing, empty, unfinished and unlinked fixture documents', async () => {
  const fixture = await mkdtemp(path.join(tmpdir(), 'synchub-npm-docs-'))
  const originals = new Map()
  try {
    for (const file of Object.keys(requirements)) {
      const content = await readFile(path.join(root, file), 'utf8')
      originals.set(file, content)
      await mkdir(path.dirname(path.join(fixture, file)), { recursive: true })
      await writeFile(path.join(fixture, file), content)
    }
    for (const file of originals.keys()) {
      const target = path.join(fixture, file)
      await rm(target)
      await assert.rejects(checkDocs(fixture), /document/i)
      await writeFile(target, '')
      await assert.rejects(checkDocs(fixture), /empty/i)
      await writeFile(target, originals.get(file) + '\nTODO: complete this\n')
      await assert.rejects(checkDocs(fixture), /unfinished/i)
      await writeFile(target, originals.get(file))
    }
    const spec = 'docs/spec.md'
    const content = originals.get(spec).replace(/## Goals[\s\S]*?(?=## Scope)/, '## Goals\n\n')
    await writeFile(path.join(fixture, spec), content)
    await assert.rejects(checkDocs(fixture), /section/i)
    await writeFile(path.join(fixture, spec), originals.get(spec).replace(
      /## Goals[\s\S]*?(?=## Scope)/,
      '## Goals\n\n<!-- visible content is missing -->\n<!-- second comment -->\n\n',
    ))
    await assert.rejects(checkDocs(fixture), /section/i)
    await writeFile(path.join(fixture, spec), originals.get(spec).replace(
      /## Goals[\s\S]*?(?=## Scope)/,
      '## Goals\n\n<!-- unterminated comment\n\n',
    ))
    await assert.rejects(checkDocs(fixture), /comment/i)
    await writeFile(path.join(fixture, spec), originals.get(spec).replace(/## Goals[\s\S]*?(?=## Scope)/, ''))
    await assert.rejects(checkDocs(fixture), /section/i)
    await writeFile(path.join(fixture, spec), originals.get(spec))
    await writeFile(path.join(fixture, 'README.md'), originals.get('README.md').replaceAll('docs/spec.md', 'unlinked.md'))
    await assert.rejects(checkDocs(fixture), /link/i)
  } finally {
    await rm(fixture, { recursive: true, force: true })
  }
})
