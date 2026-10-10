import { spawn } from 'node:child_process'
import { ensureInstalled } from './install.mjs'

export async function runCLI(args, {
  installer = ensureInstalled, spawnImpl = spawn, output = console.log,
} = {}) {
  if (args.length > 1 || (args.length === 1 && !['--install', '--version', '--hidden', '--help'].includes(args[0]))) {
    throw new Error('Usage: synchub-for-agents [--install | --version | --hidden | --help]')
  }
  if (args[0] === '--help') {
    output('Usage: synchub-for-agents [--install | --version | --hidden | --help]\nInstall downloads only. Launching waits until the desktop app exits.')
    return 0
  }
  const installed = await installer()
  if (args[0] === '--install') {
    output(`Verified SyncHub for Agents desktop ${installed.version}: ${installed.executable}`)
    return 0
  }
  return await new Promise((resolve, reject) => {
    const child = spawnImpl(installed.executable, args, { stdio: 'inherit', shell: false, windowsHide: true })
    child.once('error', reject)
    child.once('exit', (code, signal) => {
      if (signal) reject(new Error(`Desktop process ended with signal ${signal}`))
      else resolve(code ?? 1)
    })
  })
}
