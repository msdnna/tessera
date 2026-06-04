// Confirm an irreversible hard delete (the record does NOT go to the archive).
// `dialog` is naive-ui's useDialog() instance. Resolves true if confirmed.
export function confirmHardDelete(dialog, label = 'задачу') {
  return new Promise((resolve) => {
    let done = false
    const finish = (v) => {
      if (!done) {
        done = true
        resolve(v)
      }
    }
    dialog.error({
      title: 'Удалить безвозвратно?',
      content: `Это действие необратимо — ${label} нельзя будет восстановить (в архив не попадёт).`,
      positiveText: 'Удалить',
      negativeText: 'Отмена',
      onPositiveClick: () => finish(true),
      onNegativeClick: () => finish(false),
      onClose: () => finish(false),
      onMaskClick: () => finish(false),
    })
  })
}
