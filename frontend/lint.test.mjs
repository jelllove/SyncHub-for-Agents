import assert from 'node:assert/strict'
import { spawnSync } from 'node:child_process'
import path from 'node:path'
import test from 'node:test'
import { fileURLToPath } from 'node:url'
import { ESLint } from 'eslint'
import { configDefaults } from 'vitest/config'
import viteConfig from './vite.config.ts'

const frontend = fileURLToPath(new URL('.', import.meta.url))
const eslint = new ESLint({
  cwd: frontend,
  overrideConfigFile: path.join(frontend, 'eslint.config.mjs'),
})

async function lint(source, filePath = 'src/lint-fixture.ts') {
  const [result] = await eslint.lintText(source, {
    filePath: path.join(frontend, filePath),
  })
  assert.equal(result.fatalErrorCount, 0, JSON.stringify(result.messages))
  return result.messages
}

test('keeps default Vitest discovery except for standalone lint tests', () => {
  assert.equal(viteConfig.test.include, undefined)
  assert.deepEqual(viteConfig.test.exclude, [...configDefaults.exclude, 'lint.test.mjs', 'doc-drift.test.mjs', 'mcp-server.test.mjs'])
  const discovers = filePath =>
    configDefaults.include.some(pattern => path.posix.matchesGlob(filePath, pattern))
    && !viteConfig.test.exclude.some(pattern => path.posix.matchesGlob(filePath, pattern))
  for (const filePath of [
    'src/App.test.tsx',
    'tooling.test.mjs',
    'validation.test.mjs',
    'maintenance.test.mjs',
    'future-tooling.test.mjs',
  ]) {
    assert.equal(discovers(filePath), true, filePath)
  }
  assert.equal(discovers('lint.test.mjs'), false)
  assert.equal(discovers('doc-drift.test.mjs'), false)
  assert.equal(discovers('mcp-server.test.mjs'), false)
  assert.equal(discovers('node_modules/synthetic/dependency.test.mjs'), false)
})

test('accepts TypeScript types, generics, type-only imports, and satisfies', async () => {
  const messages = await lint(`
    import type { Snapshot } from '../bindings/synthetic'
    export type Names = keyof Snapshot
    export interface Entry { name: string; count?: number }
    export const initial = { name: 'synthetic' } satisfies Entry
    export const identity = <T extends Entry>(value: T): T => value
    export function count(value: Entry): number {
      return value.count ?? 0
    }
  `)
  assert.deepEqual(messages, [])
})

test('accepts TypeScript declaration merging and function/class overloads', async () => {
  const messages = await lint(`
    export interface Entry { name: string }
    export interface Entry { count: number }
    export function entry(value: string): Entry
    export function entry(value: number): Entry
    export function entry(value: string | number): Entry {
      return { name: String(value), count: 1 }
    }
    export class Counter {
      read(value: string): number
      read(value: number): number
      read(value: string | number): number { return Number(value) }
    }
  `)
  assert.deepEqual(messages, [])
})

test('preserves the compiler policy for any and unused parameters', async () => {
  const messages = await lint(`
    export function identity(value, unused: any): any { return value }
    export const options: any = {}
  `)
  assert.deepEqual(messages, [])
})

test('allows deliberate nullish equality checks', async () => {
  assert.deepEqual(await lint(`
    export function present(value: string | null | undefined): boolean {
      return value != null
    }
  `), [])
})

test('accepts typed TSX components with unconditional hooks', async () => {
  const messages = await lint(`
    import { useState } from 'react'
    type Props = { label: string }
    export function Counter({ label }: Props) {
      const [count, setCount] = useState(0)
      return <button onClick={() => setCount(count + 1)}>{label}: {count}</button>
    }
  `, 'src/LintFixture.tsx')
  assert.deepEqual(messages, [])
})

const defects = [
  ['unreachable code', 'no-unreachable', `
    export function value() { return 1; throw new Error('unreachable') }
  `],
  ['constant fallback', 'no-constant-binary-expression', `
    export const options = {} || { enabled: true }
  `],
  ['unsafe optional chaining', 'no-unsafe-optional-chaining', `
    export function name(entry?: { owner: { name: string } }) {
      return (entry?.owner).name
    }
  `],
  ['loose equality', 'eqeqeq', `
    export function equal(value: string | number) { return value == 1 }
  `],
  ['NaN comparison', 'use-isnan', `
    export function invalid(value: number) { return value === NaN }
  `],
  ['async promise executor', 'no-async-promise-executor', `
    export const result = new Promise(async (resolve) => { resolve(1) })
  `],
  ['overridden finally result', 'no-unsafe-finally', `
    export function result() {
      try { return 'success' } finally { return 'hidden failure' }
    }
  `],
  ['loop that never iterates twice', 'no-unreachable-loop', `
    export function first(values: string[]) {
      for (const value of values) { return value }
      return ''
    }
  `],
  ['function-scoped var', 'no-var', `
    var value = 1
    export { value }
  `],
  ['never-reassigned let', 'prefer-const', `
    let value = 1
    export { value }
  `],
]

for (const [description, ruleId, source] of defects) {
  test(`rejects ${description} in TypeScript`, async () => {
    const messages = await lint(source)
    assert.ok(messages.some(message => message.ruleId === ruleId && message.severity === 2),
      JSON.stringify(messages))
  })
}

test('rejects conditional React hooks in TSX', async () => {
  const messages = await lint(`
    import { useState } from 'react'
    export function Counter({ enabled }: { enabled: boolean }) {
      if (enabled) { useState(0) }
      return <span>synthetic</span>
    }
  `, 'src/LintFixture.tsx')
  assert.ok(messages.some(message =>
    message.ruleId === 'react-hooks/rules-of-hooks' && message.severity === 2),
  JSON.stringify(messages))
})

test('uses browser globals in source and Node globals in tooling', async () => {
  assert.deepEqual(await lint('export const title = document.title', 'src/lint-fixture.js'), [])
  assert.deepEqual(await lint('export const cwd = process.cwd()', 'scripts/generate-icons.mjs'), [])
  assert.deepEqual(await lint(
    'export const port: number = Number(process.env.WAILS_VITE_PORT) || 9245',
    'vite.config.ts',
  ), [])
})

test('keeps undefined-name and unused-variable checks for JavaScript tooling', async () => {
  const messages = await lint(`
    export function result() {
      const unused = 1
      return missingValue
    }
  `, 'scripts/generate-icons.mjs')
  for (const ruleId of ['no-undef', 'no-unused-vars']) {
    assert.ok(messages.some(message => message.ruleId === ruleId && message.severity === 2),
      JSON.stringify(messages))
  }
})

test('does not leak Node globals into browser JavaScript or vice versa', async () => {
  for (const [source, filePath] of [
    ['export const cwd = process.cwd()', 'src/lint-fixture.js'],
    ['export const title = document.title', 'scripts/generate-icons.mjs'],
  ]) {
    assert.ok((await lint(source, filePath)).some(message => message.ruleId === 'no-undef'))
  }
})

test('rejects unused lint suppression comments', async () => {
  const messages = await lint('/* eslint-disable eqeqeq */\nexport const value = 1')
  assert.ok(messages.some(message =>
    message.severity === 2 && message.message.includes('Unused eslint-disable')))
})

for (const filePath of [
  'bindings/synthetic.ts',
  'dist/assets/synthetic.js',
  'node_modules/synthetic/index.js',
  'tooling.test.mjs',
  'validation.test.mjs',
  'maintenance.test.mjs',
  'future-tooling.test.mjs',
]) {
  test(`ignores generated or separately owned ${filePath}`, async () => {
    assert.equal(await eslint.isPathIgnored(path.join(frontend, filePath)), true)
  })
}

for (const filePath of [
  'src/App.tsx',
  'src/App.test.tsx',
  'src/desktopState.ts',
  'src/vite-env.d.ts',
  'vite.config.ts',
  'scripts/generate-icons.mjs',
  'eslint.config.mjs',
  'lint.test.mjs',
]) {
  test(`includes handwritten ${filePath}`, async () => {
    assert.equal(await eslint.isPathIgnored(path.join(frontend, filePath)), false)
  })
}

test('the real ESLint CLI exits successfully for good TSX and fails for a defect', () => {
  for (const [source, expectedStatus] of [
    ['export function View() { return <span>synthetic</span> }', 0],
    ['export const value = {} || true', 1],
  ]) {
    const result = spawnSync(process.execPath, [
      path.join(frontend, 'node_modules', 'eslint', 'bin', 'eslint.js'),
      '--stdin',
      '--stdin-filename', path.join(frontend, 'src', 'LintFixture.tsx'),
      '--max-warnings', '0',
    ], { cwd: frontend, input: source, encoding: 'utf8', timeout: 30_000 })
    assert.equal(result.error, undefined)
    assert.equal(result.status, expectedStatus, result.stdout + result.stderr)
    if (expectedStatus === 1) {
      assert.match(result.stdout, /no-constant-binary-expression/)
    }
  }
})
