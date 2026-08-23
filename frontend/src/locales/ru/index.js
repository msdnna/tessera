// Russian bundle. Stays in the main chunk: `ru` is the default and the
// fallback locale, so it must be there before the first render (#2797).
// Namespaces mirror the feature they belong to — keep this list in sync with
// en/index.js, tests/cx-i18n.spec.js fails on any divergence.
import common from './common.json'
import shell from './shell.json'
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
import jobs from './jobs.json'
import errors from './errors.json'

export default {
  common,
  shell,
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
  jobs,
  errors,
}
