// English bundle. Imported only through the dynamic import() in src/i18n —
// that is what keeps it (and every future locale) out of the main chunk
// instead of growing the initial bundle per language (#2797).
import common from './common.json'
import shell from './shell.json'
import app from './app.json'
import board from './board.json'
import task from './task.json'
import project from './project.json'
import documents from './documents.json'
import notes from './notes.json'
import reminders from './reminders.json'
import milestones from './milestones.json'
import settings from './settings.json'
import notifications from './notifications.json'
import gitlab from './gitlab.json'
import tour from './tour.json'
import jobs from './jobs.json'
import errors from './errors.json'
import whatsNew from './whatsNew.json'

export default {
  common,
  shell,
  app,
  board,
  task,
  project,
  documents,
  notes,
  reminders,
  milestones,
  settings,
  notifications,
  gitlab,
  tour,
  jobs,
  errors,
  whatsNew,
}
