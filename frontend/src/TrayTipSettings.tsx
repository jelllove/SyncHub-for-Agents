import { useEffect, useRef, useState } from 'react'
import { Events } from '@wailsio/runtime'
import { useI18n } from './i18n'
import {
  SetTrayTipsEnabled,
  TrayTipsEnabled,
} from '../bindings/github.com/qinqingxu/synchub-for-agents/internal/desktop/wailsservice'

export function TrayTipSettings() {
  const { t } = useI18n()
  const [enabled, setEnabled] = useState<boolean>()
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [revision, setRevision] = useState(0)
  const mounted = useRef(false)

  useEffect(() => {
    mounted.current = true
    return () => { mounted.current = false }
  }, [])

  useEffect(() => {
    let active = true
    setLoading(true)
    setError('')
    void TrayTipsEnabled().then((value) => {
      if (active) setEnabled(value)
    }).catch((cause) => {
      if (active) setError(cause instanceof Error ? cause.message : String(cause))
    }).finally(() => {
      if (active) setLoading(false)
    })
    const unsubscribe = Events.On('tray-tip:preferences-changed', (event) => {
      if (!event.sender) setRevision((value) => value + 1)
    })
    return () => { active = false; unsubscribe() }
  }, [revision])

  const save = async (value: boolean) => {
    setSaving(true)
    setError('')
    try {
      await SetTrayTipsEnabled(value)
      if (mounted.current) setRevision((current) => current + 1)
    } catch (cause) {
      if (mounted.current) setError(cause instanceof Error ? cause.message : String(cause))
    } finally {
      if (mounted.current) setSaving(false)
    }
  }

  return (
    <fieldset>
      <legend>{t('Activity tips', '活动提示')}</legend>
      <label className="toggle-row">
        <span>
          <strong>{t('Show activity tips', '显示活动提示')}</strong>
          <small>{t('Show small Windows tray popups while syncing. Hide after 5 seconds, or 10 seconds for errors. Changes save immediately.', '同步时显示 Windows 托盘小弹窗。5 秒后隐藏，错误提示则在 10 秒后隐藏。更改立即保存。')}</small>
        </span>
        <input
          type="checkbox"
          checked={enabled ?? false}
          disabled={loading || saving || enabled === undefined || !!error}
          onChange={(event) => void save(event.target.checked)}
        />
      </label>
      <small>{t('The tray icon and error status stay active even when popups are off.', '关闭弹窗后，托盘图标和错误状态仍会显示。')}</small>
      {(loading || saving) && <small role="status">{saving ? t('Saving activity tip settings…', '正在保存活动提示设置…') : t('Loading activity tip settings…', '正在加载活动提示设置…')}</small>}
      {error && (
        <>
          <div className="inline-error" role="alert">{error}</div>
          <button className="secondary" onClick={() => setRevision((value) => value + 1)} type="button">
            {t('Retry activity tip settings', '重试活动提示设置')}
          </button>
        </>
      )}
    </fieldset>
  )
}
