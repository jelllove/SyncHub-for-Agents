import { readFile } from 'node:fs/promises'
import path from 'node:path'
import { pathToFileURL } from 'node:url'

export const requirements = {
  'docs/spec.md': ['Goals', 'Scope', 'Functional requirements', 'Interactions', 'Edge cases', 'Non-goals', 'Acceptance criteria'],
  'docs/arch.md': ['System boundaries', 'Module responsibilities', 'Data and state flow', 'Technology decisions', 'Error handling', 'Execution', 'Validation'],
  'README.md': ['Install and run', 'Platforms and prerequisites', 'Updates and uninstall', 'Source verification and publishing'],
  'AGENTS.md': ['Project documents', 'Implementation requirements', 'Completion requirements'],
}

export async function checkDocs(root) {
  const documents = new Map()
  for (const [file, headings] of Object.entries(requirements)) {
    let text
    try {
      text = (await readFile(path.join(root, file), 'utf8')).replaceAll('\r\n', '\n')
    } catch (cause) {
      throw new Error(`Required document is missing or unreadable: ${file}`, { cause })
    }
    if (!text.trim()) throw new Error(`Required document is empty: ${file}`)
    if (/\b(?:TODO|TBD|FIXME|PLACEHOLDER)\b/i.test(text)) throw new Error(`Unfinished documentation: ${file}`)
    const sections = [...text.matchAll(/^## (.+)$/gm)]
    for (const heading of headings) {
      const index = sections.findIndex((match) => match[1] === heading)
      if (index < 0) throw new Error(`Required section is missing: ${file} / ${heading}`)
      const content = text.slice(sections[index].index + sections[index][0].length,
        sections[index + 1]?.index ?? text.length).replace(/<!--[\s\S]*?-->/g, '').trim()
      if (!content) throw new Error(`Required section is empty: ${file} / ${heading}`)
    }
    documents.set(file, text)
  }
  for (const file of ['README.md', 'AGENTS.md']) {
    for (const target of ['docs/spec.md', 'docs/arch.md']) {
      if (!documents.get(file).includes(`](${target})`)) {
        throw new Error(`Documentation link missing: ${file} -> ${target}`)
      }
    }
  }
}

if (process.argv[1] && pathToFileURL(path.resolve(process.argv[1])).href === import.meta.url) {
  try {
    await checkDocs(path.resolve(import.meta.dirname, '..'))
    console.log('NPM package documentation: passed')
  } catch (error) {
    console.error(error.message)
    process.exitCode = 1
  }
}
