// Shared theme context of the React side. Every Vue island reads it, so one
// <TesseraProvider> at the root switches light/dark, the accent scheme and the UI
// language of every component below it at once.
import { createContext, useContext } from 'react'

export const DEFAULT_THEME = { mode: 'light', accent: 'purple', language: 'ru' }

export const TesseraContext = createContext(DEFAULT_THEME)

export function useTesseraTheme() {
  return useContext(TesseraContext)
}

// What the outermost provider wrote to :root. The app's theme store (which every
// island instantiates) also writes --t-* to :root whenever its theme changes;
// islands call reapplyRoot() after such a flush so a nested dark/accent provider
// can never repaint the page body — :root always reflects the outermost provider.
let rootState = null

export function setRootState(state) {
  rootState = state
  reapplyRoot()
}

export function reapplyRoot() {
  if (!rootState || typeof document === 'undefined') return
  const root = document.documentElement
  for (const [k, v] of Object.entries(rootState.vars)) root.style.setProperty(k, v)
  root.setAttribute('data-theme', rootState.mode)
  root.lang = rootState.language
}
