import { useMemo, useSyncExternalStore } from 'react'

export type Locale = 'en' | 'zh-CN'
export type Translator = (english: string, chinese: string) => string
export const LANGUAGE_STORAGE_KEY = 'synchub.ui.language'

type Preference = { locale: Locale; error: '' | 'read' | 'write' | 'invalid' }

export function isLocale(value: string | null): value is Locale {
  return value === 'en' || value === 'zh-CN'
}

export function resolveLocale(saved: string | null, systemLanguage: string): Locale {
  return isLocale(saved) ? saved : /^zh(?:-|$)/i.test(systemLanguage) ? 'zh-CN' : 'en'
}

function readPreference(): Preference {
  try {
    const saved = window.localStorage.getItem(LANGUAGE_STORAGE_KEY)
    const invalid = saved !== null && !isLocale(saved)
    if (invalid) console.warn('Unsupported SyncHub language preference; using the system language.')
    return { locale: resolveLocale(saved, navigator.language), error: invalid ? 'invalid' : '' }
  } catch (cause) {
    console.error('Could not read the SyncHub language preference.', cause)
    return { locale: resolveLocale(null, navigator.language), error: 'read' }
  }
}

let preference = readPreference()
const listeners = new Set<() => void>()
const getSnapshot = () => preference

function update(next: Preference) {
  document.documentElement.lang = next.locale
  if (preference.locale === next.locale && preference.error === next.error) return
  preference = next
  listeners.forEach((listener) => listener())
}

export function setLocale(locale: Locale) {
  if (!isLocale(locale)) throw new Error('Unsupported interface language.')
  let error: Preference['error'] = ''
  try {
    window.localStorage.setItem(LANGUAGE_STORAGE_KEY, locale)
  } catch (cause) {
    console.error('Could not save the SyncHub language preference.', cause)
    error = 'write'
  }
  update({ locale, error })
}

function storageChanged(event: StorageEvent) {
  if (event.key !== null && event.key !== LANGUAGE_STORAGE_KEY) return
  update(readPreference())
}

function subscribe(listener: () => void) {
  if (listeners.size === 0) {
    if (!preference.error) update(readPreference())
    window.addEventListener('storage', storageChanged)
  }
  document.documentElement.lang = preference.locale
  listeners.add(listener)
  return () => {
    listeners.delete(listener)
    if (listeners.size === 0) window.removeEventListener('storage', storageChanged)
  }
}

export function useI18n() {
  const { locale, error } = useSyncExternalStore(subscribe, getSnapshot)
  const t: Translator = useMemo(() => (english, chinese) => locale === 'zh-CN' ? chinese : english, [locale])
  return { locale, t, setLocale, error }
}
