/**
 * UserAvatar — a round member avatar: the photo when `src` loads, otherwise the
 * initials of `name` on the accent gradient. Overlapping stacks get a ring in the
 * surface colour (`stacked`).
 * @category People
 */
import { createElement } from 'react'
import UserAvatarVue from '@/components/UserAvatar.vue'
import { vueIsland } from '../runtime/island'

const Island = vueIsland(UserAvatarVue, {
  name: 'UserAvatar',
  display: 'inline-flex',
  toProps: ({ name, src }) => ({ name, src }),
})

// The app sizes avatars at the call site (.avatar on the card: 22px, 10px
// initials, accent-gradient fill); the shell applies the same metrics.
export function UserAvatar({ size = 22, stacked = false, style, className, ...rest }) {
  return createElement(Island, {
    ...rest,
    className: `tds-avatar${stacked ? ' tds-avatar-stacked' : ''}${className ? ' ' + className : ''}`,
    style: { '--tds-ava': `${size}px`, ...(style || {}) },
  })
}
