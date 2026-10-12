// Folding a call into the mini-window (v2, #3107): the session lives in the
// store, so leaving the room's route is all it takes — the floating window
// shows up on whatever screen comes next. Back to where the reader came from
// when that was somewhere else; the conferences list otherwise.
export function leaveCallView(router, conferenceId) {
  const back = typeof window !== 'undefined' ? window.history.state?.back : null
  if (back && !String(back).startsWith(`/conferences/${conferenceId}`)) router.back()
  else router.push('/conferences')
}
