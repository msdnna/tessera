// Vue-in-React bridge. Tessera's UI is Vue 3 + Naive UI; Claude Design builds with
// React. Each exported component is a thin React shell that mounts the REAL Vue
// component (the same SFC the web app ships) as its own small Vue app — an
// "island" — with the providers the app root normally gives it: Pinia, vue-i18n,
// Naive's config/message/dialog providers, and the theme store driven from the
// React <TesseraProvider>.
//
// React props flow into the island reactively (one shallowReactive bag), React
// callbacks are wired to the Vue component's emits, and React children can be
// projected into Vue slots: the slot renders an empty placeholder element and React
// portals its children into it, so composition works in both directions.
import { createElement, Fragment, useEffect, useLayoutEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import {
  camelize,
  createApp,
  defineComponent,
  h,
  shallowReactive,
  shallowRef,
  watch,
  watchEffect,
} from 'vue'
import { createPinia } from 'pinia'
import { NConfigProvider, NDialogProvider, NMessageProvider } from 'naive-ui'
import { i18n, setI18nLocale } from '@/i18n'
import { naivePack } from '@/i18n/naive'
import { COLOR_THEMES, useThemeStore } from '@/stores/theme'
import { reapplyRoot, useTesseraTheme } from './context'

const handlerKey = (ev) => {
  const c = camelize(ev)
  return 'on' + c.charAt(0).toUpperCase() + c.slice(1)
}

/**
 * @param {object} Comp  the Vue component to mount
 * @param {object} opts
 * @param {string} opts.name        CSS hook: the host gets class `tds-island tds-<name>`
 * @param {(p: object) => object} [opts.toProps]  React props → Vue props
 * @param {Record<string,string>} [opts.events]   React callback prop → Vue emit name
 * @param {Record<string,string|{name:string,box?:boolean}>} [opts.slots]  React node
 *        prop → Vue slot
 * @param {(pinia: object, p: object) => void} [opts.seed]  fill stores the component
 *        reads (re-runs when props change)
 * @param {string} [opts.display]   CSS display of the host element
 * @param {object} [opts.model]     two-way value: { vueProp, vueEvent, reactProp,
 *        defaultProp, changeProp }. Controlled when the React prop is set; otherwise
 *        the island keeps the value itself (seeded from defaultProp), so an
 *        uncontrolled input still types/toggles in a static design.
 */
export function vueIsland(Comp, opts = {}) {
  const { name, events = {}, slots = {}, seed, display = 'block', model } = opts
  // Keys that belong to the React shell, never forwarded to the Vue component.
  const reserved = new Set([
    'className',
    'style',
    ...Object.keys(events),
    ...Object.keys(slots),
    ...(model ? [model.reactProp, model.defaultProp, model.changeProp] : []),
  ])
  const toProps =
    opts.toProps || ((p) => Object.fromEntries(Object.entries(p).filter(([k]) => !reserved.has(k))))

  function Island(props) {
    const theme = useTesseraTheme()
    const hostRef = useRef(null)
    const stateRef = useRef(null)
    const [slotEls, setSlotEls] = useState({})
    if (!stateRef.current) stateRef.current = shallowReactive({ props, theme })

    useLayoutEffect(() => {
      const state = stateRef.current
      const pinia = createPinia()
      const local = shallowRef(undefined)
      const slotRefs = {}
      const slotRef = (key) =>
        (slotRefs[key] ||= (el) => {
          setSlotEls((prev) => (prev[key] === el ? prev : { ...prev, [key]: el }))
        })

      const Root = defineComponent({
        name: `TdsIsland_${name}`,
        setup() {
          const ts = useThemeStore()
          watchEffect(() => {
            ts.themeMode = state.theme.mode === 'dark' ? 'dark' : 'light'
            ts.activeTheme =
              COLOR_THEMES.find((t) => t.key === state.theme.accent) ?? COLOR_THEMES[0]
            // Dates/numbers format by the store's language, not by vue-i18n's.
            ts.language = state.theme.language
          })
          // The store's own :root write runs in the pre-flush; undo it after.
          watch(() => [ts.isDark, ts.activeTheme], reapplyRoot, { flush: 'post', immediate: true })
          watchEffect(() => {
            setI18nLocale(state.theme.language)
          })
          if (seed) watchEffect(() => seed(pinia, state.props))

          return () => {
            const p = state.props
            const vprops = { ...toProps(p) }
            for (const [rk, ev] of Object.entries(events)) {
              if (typeof p[rk] === 'function') vprops[handlerKey(ev)] = (...a) => p[rk](...a)
            }
            if (model) {
              const controlled = p[model.reactProp] !== undefined
              vprops[model.vueProp] = controlled
                ? p[model.reactProp]
                : local.value !== undefined
                  ? local.value
                  : p[model.defaultProp]
              vprops[handlerKey(model.vueEvent)] = (v) => {
                local.value = v
                if (typeof p[model.changeProp] === 'function') p[model.changeProp](v)
              }
            }
            const vslots = {}
            for (const [rk, spec] of Object.entries(slots)) {
              if (p[rk] != null && p[rk] !== false) {
                // `box` slots get a real inline box: a popover trigger is measured
                // (getBoundingClientRect), which a display:contents placeholder fails.
                const { name: sn, box } = typeof spec === 'string' ? { name: spec } : spec
                vslots[sn] = () =>
                  h('span', { ref: slotRef(rk), class: box ? 'tds-slot-box' : 'tds-slot' })
              }
            }
            const pack = naivePack(state.theme.language)
            return h(
              NConfigProvider,
              {
                abstract: true,
                theme: ts.naiveTheme,
                themeOverrides: ts.themeOverrides,
                locale: pack.locale,
                dateLocale: pack.dateLocale,
              },
              () =>
                h(NMessageProvider, null, () =>
                  h(NDialogProvider, null, () => h(Comp, vprops, vslots)),
                ),
            )
          }
        },
      })

      const app = createApp(Root)
      app.use(pinia)
      app.use(i18n)
      app.mount(hostRef.current)
      return () => {
        app.unmount()
        setSlotEls({})
      }
    }, [])

    useLayoutEffect(() => {
      const state = stateRef.current
      if (state.props !== props) state.props = props
      if (state.theme !== theme) state.theme = theme
    })

    // A popover measures its trigger when it opens — for a pinned one (`show`)
    // that is before React has portalled the trigger content into the slot, so it
    // anchors to an empty box. Naive re-syncs followers on window resize.
    useEffect(() => {
      if (!Object.keys(slotEls).length) return
      const raf = requestAnimationFrame(() => window.dispatchEvent(new Event('resize')))
      return () => cancelAnimationFrame(raf)
    }, [slotEls])

    const portals = Object.entries(slotEls)
      .filter(([k, el]) => el && props[k] != null)
      .map(([k, el]) => createPortal(props[k], el, k))

    return createElement(
      Fragment,
      null,
      createElement('div', {
        ref: hostRef,
        className: `tds-island tds-${name}${props.className ? ' ' + props.className : ''}`,
        style: { display, ...(props.style || {}) },
      }),
      ...portals,
    )
  }
  Island.displayName = name
  return Island
}
