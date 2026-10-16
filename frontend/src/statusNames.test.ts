import { describe, expect, it } from 'vitest'
import { statusName } from './statusNames'
import { activityMessage } from './activityMessages'

describe('known UI protocol translations', () => {
  const chinese = (_english: string, translated: string) => translated
  const english = (value: string) => value

  it('preserves unknown names, diagnostic substrings and inherited object keys', () => {
    for (const value of ['my-plugin', 'constructor', 'toString', 'Pulling failed: C:\\synthetic', '']) {
      expect(statusName(value, chinese)).toBe(value)
      expect(activityMessage(value, chinese)).toBe(value)
    }
  })

  it('translates exact status names without changing their English representation', () => {
    expect(statusName('dependencies', chinese)).toBe('依赖项')
    expect(statusName('instructions', chinese)).toBe('指令')
    expect(statusName('instructions', english)).toBe('instructions')
    expect(activityMessage('Pulling', chinese)).toBe('正在拉取')
    expect(activityMessage('Pulling', english)).toBe('Pulling')
  })
})
