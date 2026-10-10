const repository = 'https://github.com/jelllove/SyncHub-for-Agents'

export function safeRelativePath(value, allowEmpty = false) {
  if (allowEmpty && value === '') return value
  if (typeof value !== 'string' || !value || value.startsWith('/') ||
      /[\\:\x00-\x1f\x7f]/.test(value) || value.split('/').some((part) => part === '..' || part === '')) {
    throw new Error(`Unsafe archive path: ${String(value)}`)
  }
  return value
}

export function selectPlatform(manifest, platform = process.platform, arch = process.arch) {
  if (manifest.repository !== repository) throw new Error('Installer repository must be the official SyncHub repository')
  if (!/^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(manifest.version)) {
    throw new Error('Installer release version must be a stable semantic version')
  }
  let key
  if (platform === 'win32' && arch === 'x64') key = 'windows'
  else if (platform === 'darwin' && ['x64', 'arm64'].includes(arch)) key = 'macos'
  else if (platform === 'linux' && arch === 'x64') key = 'linux'
  else throw new Error(`Unsupported SyncHub platform: ${platform}/${arch}`)
  const asset = manifest.assets?.[key]
  if (!asset || !/^[A-Za-z0-9._-]+$/.test(asset.name) ||
      !Number.isSafeInteger(asset.size) || asset.size <= 0 || asset.size > 256 * 1024 * 1024 ||
      !/^[a-f0-9]{64}$/.test(asset.sha256) || !['zip', 'tar'].includes(asset.format)) {
    throw new Error(`Invalid pinned archive metadata for ${key}`)
  }
  safeRelativePath(asset.root, true)
  safeRelativePath(asset.executable)
  return {
    ...asset, platform, arch, version: manifest.version,
    url: `${repository}/releases/download/v${manifest.version}/${asset.name}`,
  }
}
