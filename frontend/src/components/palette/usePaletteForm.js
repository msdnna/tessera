// Palette forms (v2 interaction model, #2969 / #2982, mockup PaletteForm): the
// popover forms of the sidebar walked through without the mouse. «>Создать
// проект» turns the palette into a step mode — the command and the current
// field as chips in the input, the fields with their values listed below.
//
// The input edits the current text field; a pick field (group, project) lists
// its options instead, filtered by what is typed; «Иконка и цвет» takes ←/→ for
// the colour and ↑/↓ for the icon. Tab / ⇧Tab move between fields, ↵ creates
// from any step (defaults are filled in from the start), ⌘↵ creates and opens.
import { computed, reactive, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'
import {
  FolderOutline,
  GridOutline,
  LinkOutline,
  TextOutline,
  ColorPaletteOutline,
  AppsOutline,
} from '@vicons/ionicons5'
import { usePaletteStore } from '@/stores/palette'
import { useWorkspacesStore } from '@/stores/workspaces'
import { useTourStore } from '@/stores/tour'
import { workspaces as wsApi, projects as projApi } from '@/api'
import { makeSlug } from '@/utils/slug'
import { humanizeError } from '@/utils/errors'
import { rank } from '@/utils/fuzzy'
import { PICKER_COLORS } from '@/utils/pickerColors'
import { useToast } from '@/composables/useToast'

export const FORM_KINDS = ['project', 'board', 'group', 'workspace']

const PROJECT_ICONS = ['', 'briefcase', 'code', 'rocket', 'flask', 'star', 'book', 'bulb']
const SPACE_ICONS = ['', 'briefcase', 'home', 'code', 'rocket', 'school', 'heart', 'game']

export function usePaletteForm() {
  const { t } = useI18n()
  const route = useRoute()
  const router = useRouter()
  const palette = usePaletteStore()
  const store = useWorkspacesStore()
  const tour = useTourStore()
  const toast = useToast()

  const kind = computed(() => palette.scope?.form || null)
  const step = computed(() => palette.scope?.step ?? 0)
  const values = reactive({
    name: '',
    slug: '',
    slugTouched: false,
    group: null,
    project: null,
    color: '',
    icon: '',
  })
  const busy = ref(false)
  const error = ref('')

  // The project the reader is in — the board form's default target, and the
  // group a new project lands in («где выбран курсор в сайдбаре»).
  const currentProject = computed(
    () => store.projects.find((p) => p.slug === route.params.projectSlug) || null,
  )

  function reset() {
    values.name = ''
    values.slug = ''
    values.slugTouched = false
    values.group = currentProject.value?.group_id || null
    values.project = currentProject.value?.id || store.projects[0]?.id || null
    values.color = kind.value === 'workspace' ? PICKER_COLORS[1].hex : PICKER_COLORS[0].hex
    values.icon = kind.value === 'project' ? 'code' : ''
    error.value = ''
  }
  watch(kind, (k) => k && reset())

  const fields = computed(() => {
    switch (kind.value) {
      case 'project':
        return [
          { key: 'name', type: 'text' },
          ...(store.canManage ? [{ key: 'slug', type: 'text' }] : []),
          { key: 'group', type: 'pick' },
          { key: 'look', type: 'look' },
        ]
      case 'board':
        return [
          { key: 'name', type: 'text' },
          { key: 'project', type: 'pick' },
        ]
      case 'group':
        return [
          { key: 'name', type: 'text' },
          { key: 'group', type: 'pick' },
        ]
      case 'workspace':
        return [
          { key: 'name', type: 'text' },
          { key: 'look', type: 'look' },
        ]
      default:
        return []
    }
  })
  const field = computed(() => fields.value[step.value] || null)

  // The input shows the current text field's value; switching fields loads it.
  function loadQuery() {
    const f = field.value
    if (!f) return
    if (f.type === 'text')
      palette.query = f.key === 'slug' ? values.slug || makeSlug(values.name) : values[f.key]
    else palette.query = ''
  }
  watch([kind, step], loadQuery)
  watch(
    () => palette.query,
    (q) => {
      const f = field.value
      if (!kind.value || f?.type !== 'text') return
      if (f.key === 'slug') {
        if (q !== makeSlug(values.name)) values.slugTouched = true
        values.slug = q
      } else {
        values[f.key] = q
      }
      error.value = ''
    },
  )

  function go(i) {
    if (!kind.value) return
    const n = fields.value.length
    palette.scope = { ...palette.scope, step: Math.min(n - 1, Math.max(0, i)) }
  }
  const next = () => go(step.value + 1)
  // ⇧Tab on the first field leaves the form for the command list it came from.
  function prev() {
    if (step.value > 0) go(step.value - 1)
    else exit()
  }
  function exit() {
    palette.scope = null
    palette.query = '>'
  }

  const icons = computed(() => (kind.value === 'workspace' ? SPACE_ICONS : PROJECT_ICONS))
  function shiftColor(d) {
    const n = PICKER_COLORS.length
    const i = Math.max(
      0,
      PICKER_COLORS.findIndex((c) => c.hex === values.color),
    )
    values.color = PICKER_COLORS[(i + d + n) % n].hex
  }
  function shiftIcon(d) {
    const list = icons.value
    const i = Math.max(0, list.indexOf(values.icon))
    values.icon = list[(i + d + list.length) % list.length]
  }

  // ── labels ──
  const groupName = (id) =>
    store.groups.find((g) => g.id === id)?.name || t('shell.paletteForm.root')
  const projectName = (id) => store.projects.find((p) => p.id === id)?.name || '—'
  const colorName = computed(() => {
    const c = PICKER_COLORS.find((x) => x.hex === values.color)
    return c ? t(`common.colors.${c.key}`).toLowerCase() : ''
  })
  const iconName = computed(() => t(`common.picker.icons.${values.icon || 'initials'}`))
  const lookValue = computed(() => `${iconName.value} · ${colorName.value}`)
  const slugValue = computed(() =>
    values.slugTouched ? makeSlug(values.slug) : makeSlug(values.name),
  )

  const ICONS = {
    name: TextOutline,
    slug: LinkOutline,
    group: FolderOutline,
    project: GridOutline,
    look: ColorPaletteOutline,
  }

  function fieldRow(f, i) {
    const active = i === step.value
    const base = {
      key: `field:${f.key}`,
      icon: ICONS[f.key],
      title: t(`shell.paletteForm.fields.${f.key}`),
      run: () => (go(i), 'stay'),
    }
    switch (f.key) {
      case 'name':
        return {
          ...base,
          sub: active ? t('shell.paletteForm.typed') : '',
          value: values.name || '—',
          fieldHint: active,
        }
      case 'slug':
        return {
          ...base,
          sub: `…/project/${slugValue.value || '…'}${values.slugTouched ? '' : ` · ${t('shell.projectForm.fromName')}`}`,
          value: slugValue.value || '—',
          fieldHint: active,
        }
      case 'group':
        return {
          ...base,
          sub:
            kind.value === 'group'
              ? t('shell.paletteForm.parentSub')
              : t('shell.paletteForm.groupSub'),
          value: groupName(values.group),
          fieldHint: active,
        }
      case 'project':
        return {
          ...base,
          sub: t('shell.paletteForm.projectSub'),
          value: projectName(values.project),
          fieldHint: active,
        }
      case 'look':
        return {
          ...base,
          dot: values.color,
          sub: t('shell.paletteForm.lookSub'),
          value: lookValue.value,
          fieldHint: active,
        }
    }
    return base
  }

  function optionRows(query) {
    const f = field.value
    if (f.key === 'group') {
      const all = [{ id: null, name: t('shell.paletteForm.root') }, ...store.groups]
      return rank(all, (g) => [g.name], query.trim()).map((g) => ({
        key: `opt:${g.id ?? 'root'}`,
        icon: AppsOutline,
        title: g.name,
        hl: query.trim(),
        checked: (values.group ?? null) === (g.id ?? null),
        run: () => {
          values.group = g.id ?? null
          next()
          return 'stay'
        },
      }))
    }
    if (f.key === 'project') {
      return rank(store.projects, (p) => [p.name], query.trim()).map((p) => ({
        key: `opt:${p.id}`,
        icon: GridOutline,
        title: p.name,
        hl: query.trim(),
        sub: groupName(p.group_id),
        checked: values.project === p.id,
        run: () => {
          values.project = p.id
          return submit(false)
        },
      }))
    }
    return []
  }

  function groupsFor(query) {
    const f = field.value
    if (!f) return []
    if (f.type === 'pick') {
      return [
        {
          id: 'options',
          title: t(`shell.paletteForm.fields.${f.key}`),
          count: null,
          rows: optionRows(query),
        },
      ]
    }
    return [
      {
        id: 'fields',
        title: t(`shell.paletteForm.fieldsTitle.${kind.value}`),
        count: t('shell.paletteForm.step', { n: step.value + 1, total: fields.value.length }),
        rows: fields.value.map(fieldRow),
      },
    ]
  }

  const canSubmit = computed(() => !!values.name.trim() && !busy.value)

  async function submit(open) {
    if (!canSubmit.value) {
      if (!values.name.trim()) {
        error.value = t('shell.paletteForm.needName')
        go(0)
      }
      return 'stay'
    }
    busy.value = true
    error.value = ''
    const name = values.name.trim()
    try {
      if (kind.value === 'project') {
        const body = { name, color: values.color, icon: values.icon }
        if (values.group) body.group_id = values.group
        if (store.canManage && values.slugTouched && slugValue.value) body.slug = slugValue.value
        const res = await wsApi.createProject(store.currentId, body)
        tour.noteCreated({ projectId: res.data?.id })
        await store.refresh()
      } else if (kind.value === 'board') {
        const project = store.projects.find((p) => p.id === values.project)
        if (!project) throw new Error(t('shell.paletteForm.needProject'))
        const res = await projApi.createBoard(project.id, { name })
        tour.noteCreated({ boardId: res.data?.id })
        await store.loadBoards(project.id)
        if (open && res.data?.slug) router.push(`/project/${project.slug}/board/${res.data.slug}`)
      } else if (kind.value === 'group') {
        const body = { name }
        if (values.group) body.parent_id = values.group
        const res = await wsApi.createGroup(store.currentId, body)
        tour.noteCreated({ groupId: res.data?.id })
        await store.refresh()
      } else if (kind.value === 'workspace') {
        const w = await store.createWorkspace(name)
        const res = await wsApi.setAppearance(w.id, {
          color: values.color,
          icon: values.icon,
          icon_mode: 'badge',
        })
        store.patchWorkspace(res.data)
        if (route.params.projectSlug || route.params.id) router.push('/')
      }
      toast?.success(t(`shell.paletteForm.created.${kind.value}`, { name }))
      return 'close'
    } catch (e) {
      error.value = e.status === 409 ? t('project.slug.taken') : humanizeError(e)
      return 'stay'
    } finally {
      busy.value = false
    }
  }

  return {
    kind,
    step,
    field,
    fields,
    values,
    busy,
    error,
    groupsFor,
    next,
    prev,
    exit,
    go,
    shiftColor,
    shiftIcon,
    submit,
    canSubmit,
  }
}
