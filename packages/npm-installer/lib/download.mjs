import { createHash } from 'node:crypto'
import { createWriteStream } from 'node:fs'
import { rm } from 'node:fs/promises'
import { Readable, Transform } from 'node:stream'
import { pipeline } from 'node:stream/promises'
import { setTimeout as delay } from 'node:timers/promises'

class RequestError extends Error {
  constructor(message, transient = false, cause) {
    super(message, { cause })
    this.transient = transient
  }
}

function trustedURL(location) {
  const url = new URL(location)
  if (url.protocol !== 'https:' || url.username || url.password || url.port ||
      !['github.com', 'release-assets.githubusercontent.com', 'objects.githubusercontent.com'].includes(url.hostname)) {
    throw new RequestError('Archive redirects must stay on approved GitHub HTTPS hosts')
  }
  return url
}

async function request(location, fetchImpl, signal) {
  let url = trustedURL(location)
  for (let redirects = 0; redirects <= 5; redirects++) {
    let response
    try {
      response = await fetchImpl(url, {
        redirect: 'manual', signal,
        headers: { 'User-Agent': 'SyncHub-for-Agents-npm-installer' },
      })
    } catch (cause) {
      const code = cause.cause?.code ?? cause.code ?? ''
      const certificateFailure = /CERT|TLS_CERT|SELF_SIGNED|UNABLE_TO_VERIFY/.test(code)
      throw new RequestError('GitHub archive request failed; check your connection or proxy settings', !certificateFailure, cause)
    }
    if ([301, 302, 303, 307, 308].includes(response.status)) {
      const next = response.headers.get('location')
      await response.body?.cancel()
      if (!next) throw new RequestError('Archive redirect has no location')
      url = trustedURL(new URL(next, url))
      continue
    }
    if (response.status !== 200) {
      await response.body?.cancel()
      throw new RequestError(`GitHub archive request returned HTTP ${response.status}`,
        [500, 502, 503, 504].includes(response.status))
    }
    return response
  }
  throw new RequestError('Too many GitHub archive redirects')
}

export async function downloadArchive(asset, target, { fetchImpl = fetch, retryDelay = 500 } = {}) {
  if (!Number.isSafeInteger(asset.size) || asset.size <= 0 || asset.size > 256 * 1024 * 1024 ||
      !/^[a-f0-9]{64}$/.test(asset.sha256)) throw new Error('Invalid archive size or SHA-256')
  const signal = AbortSignal.timeout(120_000)
  let response
  for (let attempt = 1; ; attempt++) {
    try {
      response = await request(asset.url, fetchImpl, signal)
      break
    } catch (error) {
      if (!(error instanceof RequestError) || !error.transient || signal.aborted) throw error
      if (attempt === 3) throw new Error(`Archive download failed after 3 attempts: ${error.message}`, { cause: error })
      await delay(retryDelay * attempt, undefined, { signal })
    }
  }
  if (!response.body) throw new Error('Archive response has no body')
  const hash = createHash('sha256')
  let size = 0
  let complete = false
  try {
    await pipeline(
      Readable.fromWeb(response.body),
      new Transform({
        transform(chunk, _encoding, callback) {
          size += chunk.length
          if (size > asset.size) return callback(new Error('Downloaded archive exceeds its pinned size'))
          hash.update(chunk)
          callback(null, chunk)
        },
      }),
      createWriteStream(target, { flags: 'w', mode: 0o600 }),
      { signal },
    )
    if (size !== asset.size) throw new Error('Downloaded archive size differs from the pinned release')
    if (hash.digest('hex') !== asset.sha256) throw new Error('Archive SHA-256 verification failed; refusing to install')
    complete = true
  } finally {
    if (!complete) await rm(target, { force: true })
  }
}
