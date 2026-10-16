import { isLocale, useI18n } from './i18n'

export function LanguageSwitcher() {
  const { locale, setLocale, error, t } = useI18n()
  return (
    <div className="language-switcher">
      <label>
        <span>Language / 语言</span>
        <select value={locale} onChange={(event) => {
          if (isLocale(event.target.value)) setLocale(event.target.value)
        }}>
          <option value="en" lang="en">English</option>
          <option value="zh-CN" lang="zh-CN">简体中文</option>
        </select>
      </label>
      {error && (
        <div className="inline-error" role="alert">
          {error === 'write'
            ? t('Could not save the language. It will apply only for this session; select it again to retry.', '无法保存语言选择。本次仍会生效，请重新选择以重试保存。')
            : error === 'read'
              ? t('Could not load the language preference. Select a language to retry saving.', '无法读取语言偏好。请选择语言并重试保存。')
              : t('The saved language is unsupported. Please choose English or Simplified Chinese.', '保存的语言不受支持，请重新选择 English 或简体中文。')}
          <button className="text-button" type="button" onClick={() => setLocale(locale)}>
            {t('Retry saving language', '重试保存语言')}
          </button>
        </div>
      )}
    </div>
  )
}
