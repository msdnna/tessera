// Mounting the due picker (#3118) for the specs that used to drive DueEditor:
// the picker open as a desktop popover in <body>, its editing model reachable as
// `model(w)` (useDueModel — pickDay, setTime, setFreq, toggleCustomDate…), the
// emitted patches, and the grid's highlighted days.
import { mount } from '@vue/test-utils'
import { h, nextTick } from 'vue'

export async function loadPicker() {
  return (await import('@/components/due/DuePicker.vue')).default
}

export function mountPicker(DuePicker, props) {
  const w = mount(
    { render: () => h(DuePicker, { show: true, ...props }) },
    { attachTo: document.body },
  )
  return w
}

export const picker = (w, DuePicker) => w.findComponent(DuePicker)
export const model = (w, DuePicker) => picker(w, DuePicker).vm.model

// The last `apply` payload (due_date + start_date + recurrence).
export function lastApply(w, DuePicker) {
  const events = picker(w, DuePicker).emitted('apply') || []
  return events.length ? events[events.length - 1][0] : null
}
export const lastRule = (w, DuePicker) => lastApply(w, DuePicker)?.recurrence ?? null

// The day numbers the main grid marks as upcoming occurrences (`rep`), for the
// month in view; days spilling in from the neighbours (`out`) are left out.
export function highlighted() {
  return [...document.querySelectorAll('[data-testid="due-cal"] .dc-day.rep')]
    .filter((b) => !b.classList.contains('out'))
    .map((b) => Number(b.textContent.trim()))
    .sort((a, b) => a - b)
}
// Days of the month in view carrying a class.
export function daysWith(cls) {
  return [...document.querySelectorAll(`[data-testid="due-cal"] .dc-day.${cls}`)]
    .filter((b) => !b.classList.contains('out'))
    .map((b) => Number(b.textContent.trim()))
    .sort((a, b) => a - b)
}

// Walk the grid `n` months forward (it opens on the due date's month).
export async function forward(w, DuePicker, n) {
  const m = model(w, DuePicker)
  for (let i = 0; i < n; i++) {
    const d = new Date(m.view.year, m.view.month + 1, 1)
    m.view = { year: d.getFullYear(), month: d.getMonth() }
  }
  await nextTick()
}

export const day = (year, month, d) => ({ year, month, day: d })

// The «this repeat will never fire» warning of the «Повтор» panel, or null: the
// panel is opened for the look and closed again (the main grid lives behind it).
export async function deadWarning(w, DuePicker) {
  const p = picker(w, DuePicker).vm
  await p.openPane('rep')
  await nextTick()
  const el = document.querySelector('[data-testid="due-rep-warn"].dead')
  const text = el ? el.textContent.trim() : null
  p.pane = 'main'
  await nextTick()
  return text
}
