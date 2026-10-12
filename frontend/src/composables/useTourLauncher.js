import { useRouter, useRoute } from 'vue-router'
import { useTourStore } from '@/stores/tour'
import { useResponsive } from '@/composables/useResponsive'
import { chapterById } from '@/data/getStarted'
import { sGet } from '@/utils/serverStorage'

// Every door into the tour (#3000) — the checklist rows, «Помощь → Обучение»,
// «Ещё», ⌘K «Обучение», Settings → «Подсказки» — goes through here, so a chapter
// always starts on the screen its first step points at:
//   «Основы», «Знакомство», the phone's «Основы» — from «Мои задачи», as v1 did;
//   a chapter that `needs: 'board'`, or one started from a step on a board (a
//   checklist row in the middle of «Основы») — on the board opened last.
// `start` means «the guide»: «Основы» on a desktop, the phone's on a phone.
export function useTourLauncher() {
  const router = useRouter()
  const route = useRoute()
  const tour = useTourStore()
  const { isMobile } = useResponsive()

  const onBoard = () => !!(route?.params?.boardSlug || route?.params?.id)

  async function launch(id = 'start', { fromId } = {}) {
    const chId = id === 'start' ? (isMobile.value ? 'mobile' : 'basics') : id
    const ch = chapterById(chId)
    if (!ch) return false
    const from = fromId ? ch.steps.find((s) => s.id === fromId) : null
    try {
      if (ch.needs === 'board' || from?.board) {
        const last = sGet('tessera_last_board')
        if (!onBoard() && last) await router?.push(last)
      } else if (
        ['basics', 'welcome', 'mobile'].includes(chId) &&
        !from?.board &&
        route &&
        route.path !== '/'
      ) {
        await router?.push('/')
      }
    } catch {
      /* a navigation guard said no — start where we are */
    }
    return tour.start(chId, { fromId })
  }

  return { launch }
}
