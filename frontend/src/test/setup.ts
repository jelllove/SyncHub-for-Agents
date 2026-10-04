import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'

// jsdom has no native dialog implementation; browser checks cover modal behavior.
if (typeof HTMLDialogElement !== 'undefined') {
  Object.defineProperties(HTMLDialogElement.prototype, {
    showModal: {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.setAttribute('open', '')
      },
    },
    close: {
      configurable: true,
      value(this: HTMLDialogElement) {
        this.removeAttribute('open')
      },
    },
  })
}

afterEach(cleanup)
