// True while a touch drag is in progress. Sortable forces its fallback on touch
// and appends a moving clone with class `sortable-fallback`; its presence is a
// reliable "a drag is happening right now" signal. Used to suppress the
// long-press context menu while dragging a card / sidebar item on touch.
export function dragActive() {
  return !!document.querySelector('.sortable-fallback')
}
