import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { SharedLibraryPanel } from './SharedLibraryPanel'
import { LanguageSwitcher } from './LanguageSwitcher'
import { setLocale } from './i18n'

const api = vi.hoisted(() => ({ scan: vi.fn(), preview: vi.fn(), apply: vi.fn() }))
vi.mock('../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice', () => ({
  LibrarySnapshot: api.scan, PreviewLibrary: api.preview, ApplyLibrary: api.apply,
}))

const local = { kind: 'skills', name: 'review', harness: 'claude', status: 'available', path: '/fixture/claude/review' }
const shared = { ...local, harness: 'library', path: '/fixture/library/review' }

describe('guided shared library', () => {
  beforeEach(() => {
    api.scan.mockReset().mockResolvedValue({ root: '/fixture/library', entries: [local] })
    api.preview.mockReset().mockImplementation(async (request) => ({
      id: 'exact-preview', request, changes: [{ path: '/fixture/SKILL.md', action: 'create', before: '', after: 'Skill content' }],
    }))
    api.apply.mockReset().mockResolvedValue(undefined)
  })
  afterEach(() => act(() => setLocale('en')))

  it('starts an empty library at adding a local resource instead of an unusable apply form', async () => {
    render(<SharedLibraryPanel />)
    const add = await screen.findByRole('button', { name: /Add existing resources/ })
    expect(add).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('img', { name: /Local resources.*shared library.*compatible tools/i })).toBeVisible()
    expect(screen.queryByRole('combobox', { name: /Target harness/ })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Preview import' }))
    expect(api.preview).toHaveBeenCalledWith({ action: 'import', kind: 'skills', name: 'review', harness: 'claude' })
    expect(api.apply).not.toHaveBeenCalled()
  })

  it('explains missing local resources without showing empty dropdowns', async () => {
    api.scan.mockResolvedValue({ root: '/fixture/library', entries: [] })
    render(<SharedLibraryPanel />)
    expect(await screen.findByText('No supported local resources found.')).toBeVisible()
    expect(screen.getByText(/SKILL.md/)).toBeVisible()
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Preview import' })).not.toBeInTheDocument()
    expect(screen.queryByText('Manage existing copies')).not.toBeInTheDocument()
  })

  it('moves an approved import to destination selection without automatically applying it', async () => {
    api.scan.mockResolvedValueOnce({ root: '/fixture/library', entries: [local] })
      .mockResolvedValue({ root: '/fixture/library', entries: [local, shared] })
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await user.click(await screen.findByRole('button', { name: 'Preview import' }))
    await user.click(screen.getByRole('button', { name: 'Confirm import' }))
    await waitFor(() => expect(screen.getByRole('button', { name: /Use shared resources/ })).toHaveAttribute('aria-pressed', 'true'))
    expect(api.apply).toHaveBeenCalledTimes(1)
    expect(api.apply).toHaveBeenCalledWith(expect.objectContaining({ action: 'import' }), 'exact-preview')
    expect(screen.getByRole('combobox', { name: 'Use in' })).toBeVisible()
    expect(screen.queryByRole('button', { name: 'Confirm apply' })).not.toBeInTheDocument()
  })

  it('keeps an approved preview and selected target when the language changes', async () => {
    api.scan.mockResolvedValue({ root: '/fixture/library', entries: [shared] })
    const user = userEvent.setup()
    render(<><LanguageSwitcher /><SharedLibraryPanel /></>)
    await user.click(await screen.findByRole('button', { name: 'Preview apply' }))
    await user.selectOptions(screen.getByRole('combobox', { name: 'Language / 语言' }), 'zh-CN')
    expect(screen.getByRole('button', { name: '确认使用' })).toBeVisible()
    expect(screen.getByText('Skill content')).toBeVisible()
    expect(api.scan).toHaveBeenCalledTimes(1)
    expect(api.preview).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole('button', { name: '确认使用' }))
    expect(api.apply).toHaveBeenCalledWith({ action: 'apply', kind: 'skills', name: 'review', harness: 'claude' }, 'exact-preview')
  })

  it('translates actual backend replacement actions without translating file contents', async () => {
    api.scan.mockResolvedValue({ root: '/fixture/library', entries: [shared] })
    api.preview.mockImplementation(async (request) => ({
      id: 'replacement', request, changes: [{ path: '/fixture/SKILL.md', action: 'replace', before: 'Original English guidance', after: 'Changed English guidance' }],
    }))
    act(() => setLocale('zh-CN'))
    render(<SharedLibraryPanel />)
    await userEvent.click(await screen.findByRole('button', { name: '预览使用' }))
    expect(screen.getByText('替换')).toBeVisible()
    expect(screen.getByText('Original English guidance')).toBeVisible()
    expect(screen.getByText('Changed English guidance')).toBeVisible()
  })

  it.each(['restore', 'detach'])('returns to a usable task after successful %s removes the last managed copy', async (action) => {
    api.scan.mockResolvedValueOnce({ root: '/fixture/library', entries: [shared, { ...local, status: 'managed' }] })
      .mockResolvedValue({ root: '/fixture/library', entries: [shared] })
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await user.click(await screen.findByText('Manage existing copies'))
    await user.click(screen.getByRole('button', { name: action === 'restore' ? 'Restore original files' : 'Stop managing a copy' }))
    await user.click(screen.getByRole('button', { name: `Preview ${action}` }))
    await user.click(screen.getByRole('button', { name: `Confirm ${action}` }))
    await waitFor(() => expect(api.scan).toHaveBeenCalledTimes(2))
    expect(screen.getByRole('button', { name: /Use shared resources/ })).toHaveAttribute('aria-pressed', 'true')
    expect(screen.getByRole('combobox', { name: 'Use in' })).toBeVisible()
    expect(screen.queryByText('Add a resource to your shared library first.')).not.toBeInTheDocument()
  })

  it('shows one main heading and keeps source paths in an optional details section', async () => {
    api.scan.mockResolvedValue({ root: '/fixture/library', entries: [shared] })
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await screen.findByRole('button', { name: 'Preview apply' })
    expect(screen.queryByRole('heading', { name: 'One source, compatible tools' })).not.toBeInTheDocument()
    expect(screen.getByText('/fixture/library')).not.toBeVisible()
    expect(screen.getByText('/fixture/library/review')).not.toBeVisible()
    await user.click(screen.getByText('Resource details and safety'))
    expect(screen.getByText('/fixture/library')).toBeVisible()
    expect(screen.getByText('/fixture/library/review')).toBeVisible()
    expect(api.preview).not.toHaveBeenCalled()
  })

  it('places low-frequency management after the current resource action', async () => {
    api.scan.mockResolvedValue({ root: '/fixture/library', entries: [shared, { ...local, status: 'managed' }] })
    render(<SharedLibraryPanel />)
    const preview = await screen.findByRole('button', { name: 'Preview apply' })
    const management = screen.getByText('Manage existing copies')
    expect(preview.compareDocumentPosition(management) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0)
  })
})
