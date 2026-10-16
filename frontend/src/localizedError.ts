import type { Translator } from './i18n'

export class LocalizedError extends Error {
  constructor(english: string, readonly chinese: string) {
    super(english)
  }
}

export function localizedErrorMessage(error: unknown, t: Translator): string {
  if (error instanceof LocalizedError) return t(error.message, error.chinese)
  return error instanceof Error ? error.message : String(error)
}
