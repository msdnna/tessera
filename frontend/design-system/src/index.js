// Tessera design system — React entry for Claude Design (#2950). Every component
// below is the real Vue component of the Tessera web app (frontend/src), mounted
// through ./runtime/island.js; only the React shells live here.
import '@/styles/main.css'
import './styles/island.css'

// Foundation
export { TesseraProvider } from './foundation/TesseraProvider.jsx'
export { useTesseraTheme } from './runtime/context.js'
export { Icon } from './foundation/Icon.jsx'
export { TesseraIcon } from './foundation/TesseraIcon.jsx'
export { BrandLogo } from './foundation/BrandLogo.jsx'

// Primitives (Naive UI under the Tessera theme)
export { Button } from './primitives/Button.jsx'
export { Input } from './primitives/Input.jsx'
export { Select } from './primitives/Select.jsx'
export { Switch } from './primitives/Switch.jsx'
export { Checkbox } from './primitives/Checkbox.jsx'
export { Tabs } from './primitives/Tabs.jsx'
export { Tooltip } from './primitives/Tooltip.jsx'
export { Card } from './primitives/Card.jsx'
export { Modal } from './primitives/Modal.jsx'

// Board
export { TaskCard } from './board/TaskCard.jsx'
export { TaskMiniCard } from './board/TaskMiniCard.jsx'
export { ColumnHeader } from './board/ColumnHeader.jsx'

// Tags, people, navigation
export { TagPill } from './tags/TagPill.jsx'
export { TagPicker } from './tags/TagPicker.jsx'
export { UserAvatar } from './people/UserAvatar.jsx'
export { ProjectIcon } from './navigation/ProjectIcon.jsx'

// Interaction model v2 (#2969): inline edit, popover forms, menus, undo toasts
export { InlineEdit } from './interaction/InlineEdit.jsx'
export { PopoverForm } from './interaction/PopoverForm.jsx'
export { FormField } from './interaction/FormField.jsx'
export { ColorIconPicker } from './interaction/ColorIconPicker.jsx'
export { ActionMenu } from './interaction/ActionMenu.jsx'
export { UndoToast } from './interaction/UndoToast.jsx'
export { ArmButton } from './interaction/ArmButton.jsx'

// Editor, feedback
export { MarkdownEditor } from './editor/MarkdownEditor.jsx'
export { EmptyState } from './feedback/EmptyState.jsx'
export { TesseraSpinner } from './feedback/TesseraSpinner.jsx'

// Design tokens and the accent-gradient helpers the app paints with.
export { DARK, LIGHT, PRIORITY_COLORS } from '@/styles/tokens'
export { COLOR_THEMES } from '@/stores/theme'
export {
  hueGrad,
  hueGradVert,
  softFill,
  tagPillBg,
  readableHue,
  onColor,
  swatchBg,
} from '@/utils/gradient'
