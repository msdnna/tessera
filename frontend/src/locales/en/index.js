// English bundle. Imported only through the dynamic import() in src/i18n —
// that is what keeps it (and every future locale) out of the main chunk
// instead of growing the initial bundle per language (#2797).
import common from './common.json'
import shell from './shell.json'
import board from './board.json'
import task from './task.json'
import project from './project.json'
import documents from './documents.json'
import settings from './settings.json'
import notifications from './notifications.json'
import gitlab from './gitlab.json'
import jobs from './jobs.json'
import errors from './errors.json'

export default {
  common,
  shell,
  board,
  task,
  project,
  documents,
  settings,
  notifications,
  gitlab,
  jobs,
  errors,
}
