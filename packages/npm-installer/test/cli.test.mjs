import assert from 'node:assert/strict'
import { EventEmitter } from 'node:events'
import test from 'node:test'
import { runCLI } from '../lib/cli.mjs'

test('the foreground launcher preserves arguments and native exit codes without a shell', async () => {
  const calls = []
  const code = await runCLI(['--version'], {
    installer: async () => ({ executable: 'C:\\synthetic path\\SyncHub.exe', version: '0.3.5' }),
    spawnImpl: (file, args, options) => {
      calls.push({ file, args, options })
      const child = new EventEmitter()
      queueMicrotask(() => child.emit('exit', 17, null))
      return child
    },
  })
  assert.equal(code, 17)
  assert.equal(calls[0].file, 'C:\\synthetic path\\SyncHub.exe')
  assert.deepEqual(calls[0].args, ['--version'])
  assert.equal(calls[0].options.shell, false)
})

test('download-only install never starts a GUI and process errors fail explicitly', async () => {
  let launched = false
  const options = {
    installer: async () => ({ executable: 'synthetic.exe', version: '0.3.5' }),
    output: () => {},
    spawnImpl: () => { launched = true; throw new Error('cannot launch executable') },
  }
  assert.equal(await runCLI(['--install'], options), 0)
  assert.equal(launched, false)
  await assert.rejects(runCLI([], options), /cannot launch/)
  await assert.rejects(runCLI(['--install', '--version'], options), /usage|argument/i)
})
