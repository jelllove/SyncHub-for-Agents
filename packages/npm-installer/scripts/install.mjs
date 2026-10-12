import { ensureInstalled } from '../lib/install.mjs'

try {
  const installed = await ensureInstalled()
  console.log(`SyncHub for Agents desktop ${installed.version} verified. Run synchub-for-agents to open it.`)
} catch (error) {
  console.error(`SyncHub NPM installation failed: ${error instanceof Error ? error.message : String(error)}`)
  console.error('Check connectivity, supported platform and package-directory permissions, then retry synchub-for-agents --install.')
  process.exitCode = 1
}
