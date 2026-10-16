import { act, render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { SharedLibraryPanel } from './SharedLibraryPanel'

const api = vi.hoisted(() => ({
  scan: vi.fn(),
  preview: vi.fn(),
  apply: vi.fn(),
}))

vi.mock('../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice', () => ({
  LibrarySnapshot: api.scan,
  PreviewLibrary: api.preview,
  ApplyLibrary: api.apply,
}))

const entry = (kind: string, name: string, harness: string, status = 'available') => ({
  kind, name, harness, status, path: `/test/${harness}/${kind}/${name}`,
})
const entries = [
  entry('skills', 'review', 'library'),
  entry('instructions', 'global', 'library'),
  entry('skills', 'import-me', 'copilot'),
  entry('skills', 'native', 'common'),
  entry('skills', 'local', 'claude'),
  entry('instructions', 'global', 'claude'),
  entry('skills', 'review', 'claude', 'managed'),
]
const changes = [
  { path: '/test/claude/skills/review/SKILL.md', action: 'update', before: 'Old guidance', after: 'New guidance' },
  { path: '/test/claude/skills/review/old.md', action: 'delete', before: 'Removed guidance', after: '' },
]

async function ready() {
  await screen.findByRole('radio', { name: 'review · Skill · Shared library' })
}

async function chooseAction(user: ReturnType<typeof userEvent.setup>, action: string) {
  if (action === 'import' || action === 'apply') {
    await user.click(screen.getByRole('button', { name: action === 'import' ? /Add existing resources/ : /Use shared resources/ }))
    return
  }
  const summary = screen.getByText('Manage existing copies')
  if (!summary.closest('details')?.open) await user.click(summary)
  await user.click(screen.getByRole('button', { name: action === 'restore' ? 'Restore original files' : 'Stop managing a copy' }))
}

async function chooseResource(user: ReturnType<typeof userEvent.setup>, key: string) {
  const radio = screen.getAllByRole('radio').find((element) => element.getAttribute('value') === key)
  if (!radio) throw new Error(`Missing selectable resource ${key}`)
  await user.click(radio)
}

describe('SharedLibraryPanel', () => {
  beforeEach(() => {
    api.scan.mockReset().mockResolvedValue({ root: '/test/.agents/library', entries })
    api.preview.mockReset().mockImplementation(async (request) => ({ id: 'approved-plan', request, changes }))
    api.apply.mockReset().mockResolvedValue(undefined)
  })

  it('separates canonical sources, import candidates and managed outputs', async () => {
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await ready()
    await user.click(screen.getByText('Resource details and safety'))
    expect(screen.getByText('/test/.agents/library')).toBeVisible()
    expect(screen.getByText('/test/library/skills/review')).toBeVisible()
    await chooseAction(user, 'import')
    const candidates = screen.getByRole('group', { name: 'Choose an existing resource' })
    expect(within(candidates).getByRole('radio', { name: 'import-me · Skill · GitHub Copilot' })).toHaveAttribute('aria-description', '/test/copilot/skills/import-me')
    expect(within(candidates).getByRole('radio', { name: 'review · Skill · Claude Code' })).toHaveAttribute('aria-description', '/test/claude/skills/review')
    await user.click(screen.getByText('Manage existing copies'))
    expect(within(screen.getByRole('region', { name: 'Managed outputs' })).getByText('/test/claude/skills/review')).toBeVisible()
    expect(screen.getByText(/Maintain one source for skills or instructions/)).toBeVisible()
  })

  it('requires a preview and explicit confirmation with actual content and deletion counts', async () => {
    const user = userEvent.setup()
    const submit = vi.fn((event) => event.preventDefault())
    render(<form onSubmit={submit}><SharedLibraryPanel /></form>)
    await ready()
    expect(screen.queryByRole('button', { name: 'Confirm apply' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Preview apply' }))
    expect(api.preview).toHaveBeenCalledWith({ action: 'apply', kind: 'skills', name: 'review', harness: 'claude' })
    expect(api.apply).not.toHaveBeenCalled()
    expect(screen.getByText('Old guidance')).toBeVisible()
    expect(screen.getByText('New guidance')).toBeVisible()
    expect(screen.getByText('Removed guidance')).toBeVisible()
    expect(screen.getByText(/2 changes · 1 deletion/)).toBeVisible()
    for (const button of screen.getAllByRole('button')) expect(button).toHaveAttribute('type', 'button')
    await user.click(screen.getByRole('button', { name: 'Confirm apply' }))
    expect(api.apply).toHaveBeenCalledWith({ action: 'apply', kind: 'skills', name: 'review', harness: 'claude' }, 'approved-plan')
    await waitFor(() => expect(api.scan).toHaveBeenCalledTimes(2))
    expect(submit).not.toHaveBeenCalled()
    expect(screen.queryByRole('button', { name: 'Confirm apply' })).not.toBeInTheDocument()
  })

  it('offers only Claude and common native skill targets and Claude global instructions', async () => {
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await ready()
    const targets = () => within(screen.getByRole('combobox', { name: 'Use in' })).getAllByRole('option').map((option) => (option as HTMLOptionElement).value)
    expect(targets()).toEqual(['claude', 'common'])
    await user.selectOptions(screen.getByRole('combobox', { name: 'Use in' }), 'common')
    expect(screen.getByText(/common folder uses native \.agents\/skills/i)).toBeVisible()
    await chooseResource(user, 'instructions/global/library')
    expect(targets()).toEqual(['claude'])
    await user.click(screen.getByRole('button', { name: 'Preview apply' }))
    expect(api.preview).toHaveBeenLastCalledWith({ action: 'apply', kind: 'instructions', name: 'global', harness: 'claude' })
  })

  it.each(['claude', 'common', 'copilot'])('imports existing %s candidates through approval', async (harness) => {
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await ready()
    await chooseAction(user, 'import')
    const name = { claude: 'local', common: 'native', copilot: 'import-me' }[harness]
    await chooseResource(user, `skills/${name}/${harness}`)
    await user.click(screen.getByRole('button', { name: 'Preview import' }))
    expect(api.apply).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Confirm import' }))
    expect(api.apply).toHaveBeenCalledWith({ action: 'import', kind: 'skills', name, harness }, 'approved-plan')
  })

  it.each(['restore', 'detach'])('explains and requires approval for %s', async (action) => {
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await ready()
    await chooseAction(user, action)
    expect(screen.getByText(/Detach keeps target files/)).toBeVisible()
    expect(screen.getByText(/Restore returns the pre-adoption backup only when.*unmodified/)).toBeVisible()
    await user.click(screen.getByRole('button', { name: `Preview ${action}` }))
    expect(api.apply).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: `Confirm ${action}` }))
    expect(api.apply).toHaveBeenCalledWith({ action, kind: 'skills', name: 'review', harness: 'claude' }, 'approved-plan')
  })

  it.each(['target', 'resource', 'action', 'refresh'])('invalidates approval when %s changes', async (change) => {
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await ready()
    await user.click(screen.getByRole('button', { name: 'Preview apply' }))
    expect(screen.getByRole('button', { name: 'Confirm apply' })).toBeVisible()
    if (change === 'target') await user.selectOptions(screen.getByRole('combobox', { name: 'Use in' }), 'common')
    if (change === 'resource') await chooseResource(user, 'instructions/global/library')
    if (change === 'action') await chooseAction(user, 'detach')
    if (change === 'refresh') await user.click(screen.getByRole('button', { name: 'Refresh library' }))
    expect(screen.queryByRole('button', { name: /Confirm/ })).not.toBeInTheDocument()
    expect(api.apply).not.toHaveBeenCalled()
  })

  it('ignores a pending preview after selection changes', async () => {
    let resolve!: (value: unknown) => void
    api.preview.mockImplementationOnce(() => new Promise((done) => { resolve = done }))
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await ready()
    await user.click(screen.getByRole('button', { name: 'Preview apply' }))
    await user.selectOptions(screen.getByRole('combobox', { name: 'Use in' }), 'common')
    await act(async () => resolve({ id: 'stale', request: {}, changes }))
    expect(screen.queryByRole('button', { name: /Confirm/ })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Preview apply' }))
    expect(screen.getByRole('button', { name: 'Confirm apply' })).toBeVisible()
  })

  it('ignores a pending apply completion after unmount without rescanning', async () => {
    let resolve!: () => void
    api.apply.mockImplementationOnce(() => new Promise<void>((done) => { resolve = done }))
    const user = userEvent.setup()
    const view = render(<SharedLibraryPanel />)
    await ready()
    await user.click(screen.getByRole('button', { name: 'Preview apply' }))
    await user.click(screen.getByRole('button', { name: 'Confirm apply' }))
    view.unmount()
    await act(async () => resolve())
    expect(api.scan).toHaveBeenCalledTimes(1)
  })

  it('shows scan errors and retries without offering stale resources', async () => {
    api.scan.mockRejectedValueOnce(new Error('Scan unavailable'))
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    expect(await screen.findByRole('alert')).toHaveTextContent('Scan unavailable')
    expect(screen.queryByRole('button', { name: 'Preview apply' })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Refresh library' }))
    await ready()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('shows failed previews and allows retry without an approval bypass', async () => {
    api.preview.mockRejectedValueOnce(new Error('Target was modified'))
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await ready()
    await user.click(screen.getByRole('button', { name: 'Preview apply' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Target was modified')
    expect(screen.queryByRole('button', { name: /Confirm|Force/i })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Preview apply' }))
    expect(screen.getByRole('button', { name: 'Confirm apply' })).toBeVisible()
  })

  it('requires a fresh preview after a failed apply', async () => {
    api.apply.mockRejectedValueOnce(new Error('Preview is stale'))
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await ready()
    await user.click(screen.getByRole('button', { name: 'Preview apply' }))
    await user.click(screen.getByRole('button', { name: 'Confirm apply' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Preview is stale')
    expect(screen.queryByRole('button', { name: /Confirm|Force/i })).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Preview apply' }))
    expect(screen.getByRole('button', { name: 'Confirm apply' })).toBeVisible()
  })

  it('does not offer unsupported global instruction mappings', async () => {
    api.scan.mockResolvedValue({
      root: '/test/.agents/library',
      entries: [
        ...entries,
        entry('instructions', 'global', 'common'),
        entry('instructions', 'global', 'copilot'),
        entry('instructions', 'project', 'library'),
        entry('skills', 'edited', 'common', 'modified'),
      ],
    })
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await ready()
    await chooseAction(user, 'import')
    expect(screen.queryByRole('radio', { name: /global · Global instructions · (Common|GitHub)/ })).not.toBeInTheDocument()
    await chooseResource(user, 'instructions/global/claude')
    await user.click(screen.getByRole('button', { name: 'Preview import' }))
    expect(api.preview).toHaveBeenLastCalledWith({ action: 'import', kind: 'instructions', name: 'global', harness: 'claude' })
  })

  it('keeps zero-file-change detach operations behind explicit confirmation', async () => {
    api.preview.mockImplementation(async (request) => ({ id: 'detach-plan', request, changes: [] }))
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await ready()
    await chooseAction(user, 'detach')
    await user.click(screen.getByRole('button', { name: 'Preview detach' }))
    expect(screen.getByText('0 changes · 0 deletions')).toBeVisible()
    expect(api.apply).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Confirm detach' }))
    expect(api.apply).toHaveBeenCalledWith(expect.objectContaining({ action: 'detach' }), 'detach-plan')
  })

  it('bounds preview text and lets the user expand the complete content', async () => {
    api.preview.mockImplementation(async (request) => ({
      id: 'large-plan', request,
      changes: [{ path: '/test/SKILL.md', action: 'update', before: '', after: 'a'.repeat(4000) + 'END OF GUIDANCE' }],
    }))
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await ready()
    await user.click(screen.getByRole('button', { name: 'Preview apply' }))
    expect(screen.queryByText('END OF GUIDANCE')).not.toBeInTheDocument()
    await user.click(screen.getByText(/Show remaining after content/))
    expect(screen.getByText('END OF GUIDANCE')).toBeVisible()
  })

  it('ignores an obsolete rejected preview after refresh', async () => {
    let reject!: (reason: Error) => void
    api.preview.mockImplementationOnce(() => new Promise((_, fail) => { reject = fail }))
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await ready()
    await user.click(screen.getByRole('button', { name: 'Preview apply' }))
    await user.click(screen.getByRole('button', { name: 'Refresh library' }))
    await ready()
    await act(async () => reject(new Error('Obsolete failure')))
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Preview apply' })).toBeEnabled()
  })

  it('ignores pending preview results after unmount and remount', async () => {
    let resolve!: (value: unknown) => void
    api.preview.mockImplementationOnce(() => new Promise((done) => { resolve = done }))
    const user = userEvent.setup()
    const old = render(<SharedLibraryPanel />)
    await ready()
    await user.click(screen.getByRole('button', { name: 'Preview apply' }))
    old.unmount()
    render(<SharedLibraryPanel />)
    await ready()
    await act(async () => resolve({ id: 'stale-plan', request: {}, changes }))
    expect(screen.queryByRole('button', { name: /Confirm/ })).not.toBeInTheDocument()
  })

  it('shows blocked legacy sources with actionable reasons but never offers them for import or apply', async () => {
    api.scan.mockResolvedValue({
      root: '/test/.agents/library',
      entries: [
        ...entries,
        entry('skills', 'legacy', 'common', 'blocked: Review the directory alias and replace it with a real directory'),
        entry('skills', 'blocked-source', 'library', 'blocked: Unsupported symbolic link'),
      ],
    })
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await ready()
    const blocked = screen.getByRole('region', { name: 'Blocked resources' })
    expect(within(blocked).getByText(/Review the directory alias/)).toBeVisible()
    expect(within(blocked).getByText(/Unsupported symbolic link/)).toBeVisible()
    expect(screen.queryByRole('radio', { name: /blocked-source/ })).not.toBeInTheDocument()
    await chooseAction(user, 'import')
    expect(screen.queryByRole('radio', { name: /legacy/ })).not.toBeInTheDocument()
  })

  it.each(['managed', 'modified'])('allows explicitly reviewed import of %s output before detach', async (status) => {
    api.scan.mockResolvedValue({
      root: '/test/.agents/library',
      entries: [...entries, entry('skills', 'edited', 'common', status)],
    })
    const user = userEvent.setup()
    render(<SharedLibraryPanel />)
    await ready()
    await chooseAction(user, 'import')
    await chooseResource(user, 'skills/edited/common')
    await user.click(screen.getByRole('button', { name: 'Preview import' }))
    expect(api.preview).toHaveBeenLastCalledWith({ action: 'import', kind: 'skills', name: 'edited', harness: 'common' })
    expect(api.apply).not.toHaveBeenCalled()
    await user.click(screen.getByRole('button', { name: 'Confirm import' }))
    await waitFor(() => expect(api.scan).toHaveBeenCalledTimes(2))
    await chooseAction(user, 'detach')
    await chooseResource(user, 'skills/edited/common')
    await user.click(screen.getByRole('button', { name: 'Preview detach' }))
    expect(api.apply).toHaveBeenCalledTimes(1)
    await user.click(screen.getByRole('button', { name: 'Confirm detach' }))
    expect(api.apply).toHaveBeenLastCalledWith({ action: 'detach', kind: 'skills', name: 'edited', harness: 'common' }, 'approved-plan')
  })

  it('keeps complete 64-character names inside the library-specific wrapping container', async () => {
    const longName = 'x'.repeat(64)
    api.scan.mockResolvedValue({
      root: '/test/.agents/library',
      entries: [...entries, entry('skills', longName, 'library')],
    })
    render(<SharedLibraryPanel />)
    await ready()
    expect(screen.getByRole('radio', { name: `${longName} · Skill · Shared library` })).toBeInTheDocument()
    const panel = screen.getByRole('heading', { name: 'Shared library' }).closest('.settings-panel')!
    expect(panel).toHaveClass('shared-library-panel')
  })
})
