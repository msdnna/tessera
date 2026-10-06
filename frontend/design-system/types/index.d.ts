// Public API of the Tessera design system (React side). Hand-written: the
// components are Vue SFCs bridged through runtime/island.js, so there is no TSX
// source to derive these from — keep them in step with the Vue `defineProps`.
// Object shapes are written inline in every prop (not as shared named types): the
// Claude Design converter copies each <Name>Props body verbatim, and a named type
// would dangle in the emitted per-component .d.ts.
import type { CSSProperties, ReactElement, ReactNode } from 'react'

/** Props every island component accepts on its React host element. */
export interface IslandHostProps {
  /** Extra class on the island's host element — layout only, never restyle internals. */
  className?: string
  /** Inline style on the island's host element — layout only (margin, width, flex). */
  style?: CSSProperties
}

// ── Foundation ──────────────────────────────────────────────────────────────

export interface TesseraProviderProps {
  /** Colour mode. */
  mode?: 'light' | 'dark'
  /** Accent scheme; drives --t-primary and every accent gradient. Default purple #7c5cff. */
  accent?: 'purple' | 'blue' | 'teal' | 'green' | 'orange' | 'red' | 'pink'
  /** Language of the built-in UI strings. Russian is the product default. */
  language?: 'ru' | 'en'
  children?: ReactNode
}
export declare function TesseraProvider(props: TesseraProviderProps): ReactElement

/** Current theme of the nearest TesseraProvider. */
export declare function useTesseraTheme(): {
  mode: 'light' | 'dark'
  accent: 'purple' | 'blue' | 'teal' | 'green' | 'orange' | 'red' | 'pink'
  language: 'ru' | 'en'
}

export interface IconProps extends IslandHostProps {
  /** ionicons5 export name, outline style as in the app: 'AddOutline', 'CalendarClearOutline'. */
  name: string
  /** Size in px (default 16). */
  size?: number | string
  /** Flat colour (any CSS colour / var). Ignored when `gradient`. */
  color?: string
  /** Paint with the accent gradient (accent/active icons). */
  gradient?: boolean
}
export declare function Icon(props: IconProps): ReactElement

export interface TesseraIconProps extends IslandHostProps {
  /** Glyph: layout-kanban|list|calendar|timeline|gantt|matrix, status-todo|progress|review|done, task-modal|sidebar|fullscreen, align-*. */
  name: string
  variant?: 'outline' | 'sharp' | 'filled'
  /** Size in px; omit to inherit font-size. */
  size?: number | string
}
export declare function TesseraIcon(props: TesseraIconProps): ReactElement

export interface BrandLogoProps extends IslandHostProps {
  /** Box height in px (default 22). */
  height?: number
  /** Show the "t" tile mark. */
  mark?: boolean
  /** Show the "tessera" wordmark. */
  wordmark?: boolean
}
export declare function BrandLogo(props: BrandLogoProps): ReactElement

// ── Primitives (Naive UI under the Tessera theme) ───────────────────────────

export interface ButtonProps extends IslandHostProps {
  /** Visual role. `primary` = the accent action. */
  type?: 'default' | 'tertiary' | 'primary' | 'info' | 'success' | 'warning' | 'error'
  size?: 'tiny' | 'small' | 'medium' | 'large'
  /** Quiet toolbar button: no border, hover tint only. */
  quaternary?: boolean
  /** Quiet button with a faint resting tint. */
  tertiary?: boolean
  /** Text-only, link-style. */
  text?: boolean
  /** Transparent body, coloured border + text. */
  ghost?: boolean
  /** Dashed border (add-column / add-item affordances). */
  dashed?: boolean
  secondary?: boolean
  /** Round icon button. */
  circle?: boolean
  round?: boolean
  block?: boolean
  strong?: boolean
  disabled?: boolean
  loading?: boolean
  /** Leading icon, usually <Icon name="…Outline" />. */
  icon?: ReactNode
  children?: ReactNode
  onClick?: (e: MouseEvent) => void
}
export declare function Button(props: ButtonProps): ReactElement

export interface InputProps extends IslandHostProps {
  /** Controlled value. */
  value?: string
  /** Initial value when uncontrolled. */
  defaultValue?: string
  onChange?: (value: string) => void
  placeholder?: string
  type?: 'text' | 'password' | 'textarea'
  size?: 'tiny' | 'small' | 'medium' | 'large'
  /** Textarea auto-height, e.g. { minRows: 2, maxRows: 6 }. */
  autosize?: boolean | { minRows?: number; maxRows?: number }
  clearable?: boolean
  round?: boolean
  disabled?: boolean
  /** Leading content (an <Icon />). */
  prefix?: ReactNode
  /** Trailing content. */
  suffix?: ReactNode
  onFocus?: (e: FocusEvent) => void
  onBlur?: (e: FocusEvent) => void
  onKeyup?: (e: KeyboardEvent) => void
}
export declare function Input(props: InputProps): ReactElement

export interface SelectProps extends IslandHostProps {
  options: { label: string; value: string | number; disabled?: boolean }[]
  /** Controlled value (array when `multiple`). */
  value?: string | number | (string | number)[] | null
  defaultValue?: string | number | (string | number)[] | null
  onChange?: (value: string | number | (string | number)[] | null) => void
  placeholder?: string
  multiple?: boolean
  filterable?: boolean
  clearable?: boolean
  size?: 'tiny' | 'small' | 'medium' | 'large'
  disabled?: boolean
}
export declare function Select(props: SelectProps): ReactElement

export interface SwitchProps extends IslandHostProps {
  value?: boolean
  defaultValue?: boolean
  onChange?: (value: boolean) => void
  size?: 'small' | 'medium' | 'large'
  disabled?: boolean
}
export declare function Switch(props: SwitchProps): ReactElement

export interface CheckboxProps extends IslandHostProps {
  checked?: boolean
  defaultChecked?: boolean
  onChange?: (checked: boolean) => void
  size?: 'small' | 'medium' | 'large'
  disabled?: boolean
  indeterminate?: boolean
  /** Label. */
  children?: ReactNode
}
export declare function Checkbox(props: CheckboxProps): ReactElement

export interface TabsProps extends IslandHostProps {
  tabs: { name: string; label: string; disabled?: boolean }[]
  /** Active tab name (controlled). Defaults to the first tab. */
  value?: string
  defaultValue?: string
  onChange?: (name: string) => void
  /** `line` = accent underline (app default); `segment` = raised pill in a track. */
  type?: 'line' | 'bar' | 'card' | 'segment'
  size?: 'small' | 'medium' | 'large'
  justifyContent?: 'start' | 'center' | 'end' | 'space-between' | 'space-around' | 'space-evenly'
}
export declare function Tabs(props: TabsProps): ReactElement

export interface TooltipProps extends IslandHostProps {
  /** The hint (text or nodes). */
  content: ReactNode
  /** The trigger element. */
  children: ReactNode
  placement?:
    | 'top'
    | 'bottom'
    | 'left'
    | 'right'
    | 'top-start'
    | 'top-end'
    | 'bottom-start'
    | 'bottom-end'
  /** Pin open (true) / closed (false); omit for hover. */
  show?: boolean
  trigger?: 'hover' | 'click' | 'focus' | 'manual'
}
export declare function Tooltip(props: TooltipProps): ReactElement

export interface CardProps extends IslandHostProps {
  title?: string
  /** Right side of the title row. */
  headerExtra?: ReactNode
  children?: ReactNode
  footer?: ReactNode
  /** Bottom action strip (on the raised surface colour). */
  action?: ReactNode
  size?: 'small' | 'medium' | 'large' | 'huge'
  bordered?: boolean
  hoverable?: boolean
  /** Divide header/content/footer — avoid: Tessera separates by surface, not lines. */
  segmented?: boolean | { content?: boolean; footer?: boolean }
}
export declare function Card(props: CardProps): ReactElement

export interface ModalProps extends IslandHostProps {
  /** Open state (controlled). */
  show?: boolean
  /** Initial open state when uncontrolled. */
  defaultShow?: boolean
  /** Called with `false` when the user closes it (✕, Esc, backdrop). */
  onClose?: (show: boolean) => void
  title?: string
  /** Dialog width (px number or CSS length). Default 560. */
  width?: number | string
  children?: ReactNode
  footer?: ReactNode
  headerExtra?: ReactNode
  closable?: boolean
  maskClosable?: boolean
  bordered?: boolean
  size?: 'small' | 'medium' | 'large' | 'huge'
}
export declare function Modal(props: ModalProps): ReactElement

// ── Board ───────────────────────────────────────────────────────────────────

export interface TaskCardProps extends IslandHostProps {
  /** The task. Ids in tag_ids / assignee_ids / column_id / milestone_id resolve against the board props. */
  task: {
    id: string
    number?: number
    title: string
    /** 0 none · 1 low · 2 normal · 3 high · 4 urgent (left bar + flag colour). */
    priority?: 0 | 1 | 2 | 3 | 4
    /** ISO date or datetime. */
    due_date?: string | null
    start_date?: string | null
    completed_at?: string | null
    column_id?: string
    tag_ids?: string[]
    assignee_ids?: string[]
    created_by?: string
    milestone_id?: string | null
    /** Hours. */
    estimate?: number | null
    has_description?: boolean
    parent_id?: string | null
  }
  /** Child tasks (same shape), listed under the card. */
  subtasks?: {
    id: string
    title: string
    priority?: number
    column_id?: string
    completed_at?: string | null
    due_date?: string | null
    assignee_ids?: string[]
  }[]
  /** Real subtask count when `subtasks` is filtered ("2 из 5"). */
  subtasksTotal?: number
  /** Subtasks as full property cards instead of compact rows. */
  subtasksExpanded?: boolean
  /** This card is a subtask shown under its parent (darker, no cascade). */
  nested?: boolean
  /** Archive view: display-only with a Restore action. */
  readonly?: boolean
  /** Picked from the keyboard (the board's arrow-key selection): an accent wash and
   * a 2px accent ring inside the edge. The board shows it only while the keyboard
   * is in use — never set it for a pointer click or on a phone. */
  selected?: boolean
  /** Board tags (TagPill shape) resolved from task.tag_ids. */
  tags?: { id: string; name: string; color?: string }[]
  /** Workspace members resolved from task.assignee_ids / created_by. */
  members?: { user_id: string; name: string; email?: string; avatar?: string }[]
  /** Board columns { id, name, color }. */
  columns?: { id: string; name: string; color?: string }[]
  /** Project milestones («Этап»). */
  milestones?: { id: string; title: string; due_date?: string | null }[]
  /** Tag scope prefix → friendly label, keyed with the separator: { 'app::': 'Приложение' }. */
  prefixNames?: Record<string, string>
  /** Card density preset (board "Customize view"). */
  cardSize?: 'compact' | 'medium' | 'large'
  /** Hide individual card fields. */
  fields?: {
    priority?: boolean
    due?: boolean
    assignee?: boolean
    tags?: boolean
    estimate?: boolean
    milestone?: boolean
    description?: boolean
    number?: boolean
    gitlab?: boolean
  }
  /** Show placeholder pills for unset fields (default true). */
  showEmpty?: boolean
  /** Stack pills vertically instead of wrapping. */
  stackFields?: boolean
  /** Card clicked — open the task. */
  onOpen?: (task: { id: string }) => void
  onChanged?: (...args: unknown[]) => void
  onRestore?: (...args: unknown[]) => void
}
export declare function TaskCard(props: TaskCardProps): ReactElement

export interface TaskMiniCardProps extends IslandHostProps {
  task: {
    id: string
    number?: number
    title: string
    priority?: 0 | 1 | 2 | 3 | 4
    due_date?: string | null
    completed_at?: string | null
    tag_ids?: string[]
    assignee_ids?: string[]
  }
  tags?: { id: string; name: string; color?: string }[]
  members?: { user_id: string; name: string; avatar?: string }[]
  prefixNames?: Record<string, string>
  /** The task's column, when a status chip should be explained. */
  column?: { id: string; name: string; color?: string } | null
}
export declare function TaskMiniCard(props: TaskMiniCardProps): ReactElement

export interface ColumnHeaderProps extends IslandHostProps {
  /** The column: name + colour (tints the status glyph and the lane). A tag column
   * (board grouped by tag) passes `tag` and is titled with that tag's pill. */
  column: { id: string; name: string; color?: string; tag?: { name: string; color?: string } }
  /** Cards in the column. */
  count?: number
  /** Cards incl. subtasks — shown as "5 (12)" when larger than count. */
  total?: number
  /** Σ estimate label, e.g. "12 ч"; empty hides it. */
  estimate?: string
  /** Status column (every real board column): status glyph + rename/colour/delete menu. */
  editable?: boolean
  /** The board's done column (filled check glyph). */
  isDone?: boolean
  /** Leftmost column (empty-circle "to do" glyph). */
  first?: boolean
  /** The column is collapsed (the board then draws the narrow strip itself). */
  collapsed?: boolean
  /** Friendly scope names for a tag column's pill, keyed WITH the separator: `{ 'app::': 'App' }`. */
  prefixNames?: Record<string, string>
  onSetDone?: (columnId: string | null) => void
  onToggleCollapse?: () => void
  onChanged?: () => void
}
export declare function ColumnHeader(props: ColumnHeaderProps): ReactElement

// ── Tags, people, navigation ────────────────────────────────────────────────

export interface TagPillProps extends IslandHostProps {
  /** Tag `{ name, color }` (name may be `scope::name`), or a bare name + `color`. */
  tag: { id?: string; name: string; color?: string } | string
  /** Colour (#RRGGBB) when `tag` is a string. */
  color?: string
  /** outline (default, cards) · solid (selected chip) · soft (unselected chip) · ghost · grad-text · plain · inherit. */
  variant?: 'outline' | 'solid' | 'soft' | 'ghost' | 'grad-text' | 'plain' | 'inherit'
  /** 'hide' drops the `scope::` segment (inside a list already grouped by scope). */
  scopeMode?: 'auto' | 'hide'
  /** Scope → friendly label, keyed with the separator: { 'app::': 'Приложение' }. */
  prefixNames?: Record<string, string>
}
export declare function TagPill(props: TagPillProps): ReactElement

export interface UserAvatarProps extends IslandHostProps {
  /** Full name — initials are drawn from it when there is no photo. */
  name: string
  /** Photo URL. */
  src?: string
  /** Diameter in px (default 22, the board card size). */
  size?: number
  /** Part of an overlapping stack: surface-coloured ring + negative margin. */
  stacked?: boolean
}
export declare function UserAvatar(props: UserAvatarProps): ReactElement

export interface ProjectIconProps extends IslandHostProps {
  /** Curated key (briefcase, home, code, rocket, school, cart, heart, star, flag, construct, book, flask, bulb, game, airplane, wallet), SVG markup or data: image. */
  icon?: string
  /** Fallback text when there is no icon. */
  initials?: string
  /** Glyph size in px (sidebar: 13). */
  size?: number
  /** Tint the glyph itself (icon colour mode); omit inside a coloured badge. */
  color?: string
}
export declare function ProjectIcon(props: ProjectIconProps): ReactElement

// ── Editor, feedback ────────────────────────────────────────────────────────

export interface MarkdownEditorProps extends IslandHostProps {
  /** Markdown (controlled). */
  value?: string
  defaultValue?: string
  onChange?: (markdown: string) => void
  placeholder?: string
  /** `default` = description editor with Write/Preview tabs; `boxed` = framed comment composer. */
  variant?: 'default' | 'boxed'
  /** Boxed: show the send button (fires onSubmit, also Ctrl+Enter). */
  send?: boolean
  /** Boxed + send: posting in progress. */
  sending?: boolean
  onSubmit?: () => void
  /** Open on the rendered preview instead of the textarea. */
  initialMode?: 'write' | 'preview'
  minRows?: number
  /** Default variant: show the formatting toolbar (default true). */
  toolbar?: boolean
  /** Text and live preview side by side. */
  split?: boolean
  /** @-mention candidates. */
  mentionItems?: { id: string; label: string; display?: string }[]
}
export declare function MarkdownEditor(props: MarkdownEditorProps): ReactElement

export interface EmptyStateProps extends IslandHostProps {
  /** ionicons5 export name (default FileTrayOutline). */
  icon?: string
  text?: string
  size?: 'small' | 'medium'
}
export declare function EmptyState(props: EmptyStateProps): ReactElement

export interface TesseraSpinnerProps extends IslandHostProps {
  /** Size in px (default 32). */
  size?: number
  /** Caption under the loader. */
  label?: string
  /** `accent` follows --t-primary; `white` for coloured backdrops. */
  variant?: 'accent' | 'white'
}
export declare function TesseraSpinner(props: TesseraSpinnerProps): ReactElement

// ── Tokens and accent-gradient helpers ──────────────────────────────────────

/** Neutral palette per mode (bg, surface, surfaceAlt, cardSurface, inputBg, hover, border, text1-3, placeholder). */
export declare const DARK: Record<string, string>
export declare const LIGHT: Record<string, string>
/** Priority colours, index = priority 0..4 (none, low, normal, high, urgent). */
export declare const PRIORITY_COLORS: string[]
/** The seven accent schemes: { key, primary, hover, pressed, suppl }. */
export declare const COLOR_THEMES: {
  key: string
  primary: string
  hover: string
  pressed: string
  suppl: string
}[]
/** Soft same-hue diagonal gradient of a colour (dark bottom-left → base centre → light top-right). */
export declare function hueGrad(color: string): string
/** Vertical variant (dark bottom → light top) for tall narrow accents. */
export declare function hueGradVert(color: string): string
/** Faint same-hue wash for tinted backgrounds. */
export declare function softFill(color: string): string
/** Background of an outline tag pill. */
export declare function tagPillBg(color: string, filled?: boolean): string
/** A hue clamped to a legible lightness as text on the given mode. */
export declare function readableHue(hex: string, isDark: boolean): string
/** Readable text colour (#fff / dark) on a fill of `hex`. */
export declare function onColor(hex: string): string
/** Background of a colour swatch. */
export declare function swatchBg(color: string): string
