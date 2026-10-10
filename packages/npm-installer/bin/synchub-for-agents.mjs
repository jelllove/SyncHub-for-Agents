#!/usr/bin/env node
import { runCLI } from '../lib/cli.mjs'

try {
  process.exitCode = await runCLI(process.argv.slice(2))
} catch (error) {
  console.error(`SyncHub for Agents: ${error instanceof Error ? error.message : String(error)}`)
  process.exitCode = 1
}
