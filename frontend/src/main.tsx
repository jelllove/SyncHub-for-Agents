import React from 'react'
import ReactDOM from 'react-dom/client'
import App from './App'
import { TrayTip } from './TrayTip'

const trayTip = new URLSearchParams(window.location.search).get('view') === 'tray-tip'
if (trayTip) {
  document.documentElement.classList.add('tray-tip-window')
  document.body.classList.add('tray-tip-window')
}
ReactDOM.createRoot(document.getElementById('root') as HTMLElement).render(
  <React.StrictMode>
    {trayTip ? <TrayTip /> : <App />}
  </React.StrictMode>,
)
