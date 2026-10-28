import { act, render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { LANGUAGE_STORAGE_KEY, resolveLocale, setLocale, useI18n } from './i18n'
import { LanguageSwitcher } from './LanguageSwitcher'

function Example() {
  const { t } = useI18n()
  return <><LanguageSwitcher /><h1>{t('Your shared library', '你的共享库')}</h1></>
}

afterEach(() => {
  vi.restoreAllMocks()
  act(() => setLocale('en'))
})

describe('language preference', () => {
  it.each([
    ['en', 'zh-CN', 'en'],
    ['zh-CN', 'en-US', 'zh-CN'],
    [null, 'zh-TW', 'zh-CN'],
    [null, 'en-US', 'en'],
    ['unsupported', 'fr-FR', 'en'],
  ])('resolves saved %s and system %s to %s', (saved, system, expected) => {
    expect(resolveLocale(saved, system)).toBe(expected)
  })

  it('switches mounted consumers immediately and remembers the choice after remount', async () => {
    const user = userEvent.setup()
    const view = render(<Example />)
    await user.selectOptions(screen.getByRole('combobox', { name: 'Language / 语言' }), 'zh-CN')
    expect(screen.getByRole('heading', { name: '你的共享库' })).toBeVisible()
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe('zh-CN')
    expect(document.documentElement.lang).toBe('zh-CN')
    view.unmount()
    render(<Example />)
    expect(screen.getByRole('combobox')).toHaveValue('zh-CN')
    await user.selectOptions(screen.getByRole('combobox'), 'en')
    expect(screen.getByRole('heading', { name: 'Your shared library' })).toBeVisible()
    expect(document.documentElement.lang).toBe('en')
  })

  it('updates another mounted window consumer from a storage event', () => {
    render(<Example />)
    act(() => {
      localStorage.setItem(LANGUAGE_STORAGE_KEY, 'zh-CN')
      window.dispatchEvent(new StorageEvent('storage', { key: LANGUAGE_STORAGE_KEY, newValue: 'zh-CN' }))
    })
    expect(screen.getByRole('heading', { name: '你的共享库' })).toBeVisible()
  })

  it('ignores storage events for unrelated preferences', () => {
    render(<Example />)
    act(() => window.dispatchEvent(new StorageEvent('storage', { key: 'another.preference', newValue: 'zh-CN' })))
    expect(screen.getByRole('heading', { name: 'Your shared library' })).toBeVisible()
  })

  it('reports storage write failure while keeping the chosen language for this session', async () => {
    const user = userEvent.setup()
    render(<Example />)
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('Storage denied') })
    const log = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    await user.selectOptions(screen.getByRole('combobox'), 'zh-CN')
    expect(screen.getByRole('heading', { name: '你的共享库' })).toBeVisible()
    expect(screen.getByRole('alert')).toHaveTextContent('无法保存语言选择')
    expect(log).toHaveBeenCalled()
    vi.restoreAllMocks()
    await user.click(screen.getByRole('button', { name: '重试保存语言' }))
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(localStorage.getItem(LANGUAGE_STORAGE_KEY)).toBe('zh-CN')
  })

  it('surfaces a failed preference read when the first consumer subscribes', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('Read denied') })
    const log = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    render(<Example />)
    expect(screen.getByRole('alert')).toHaveTextContent('Could not load the language preference')
    expect(log).toHaveBeenCalled()
  })
})
