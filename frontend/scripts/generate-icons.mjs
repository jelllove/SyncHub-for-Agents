import { mkdir } from 'node:fs/promises'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import sharp from 'sharp'

const frontend = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')
const root = path.resolve(frontend, '..')
const sources = path.join(root, 'assets', 'icons')
const trayOutput = path.join(root, 'internal', 'tray', 'assets')

await mkdir(trayOutput, { recursive: true })

await sharp(path.join(sources, 'agentconfigsync.svg'), { density: 384 })
  .resize(1024, 1024)
  .png({ compressionLevel: 9 })
  .toFile(path.join(root, 'build', 'appicon.png'))

for (const state of ['ready', 'updating', 'done', 'error', 'paused']) {
  for (const size of [16, 20, 24, 32]) {
    await sharp(path.join(sources, `tray-${state}.svg`), { density: 384 })
      .resize(size, size)
      .png({ compressionLevel: 9 })
      .toFile(path.join(trayOutput, `${state}-${size}.png`))
  }
}
