// How a workspace looks on the rail (#2955): its own colour, else one derived
// from the id (stable across sessions and clients); initials of its caption.
import { userColor } from '@/utils/userColor'
import { initials } from '@/utils/initials'
import { workspaceName } from '@/utils/defaultNames'

export function tileColor(w) {
  return w?.color && w.color !== 'transparent' ? w.color : userColor(w?.id || '')
}

export function tileLabel(w) {
  return initials(workspaceName(w)) || '?'
}
