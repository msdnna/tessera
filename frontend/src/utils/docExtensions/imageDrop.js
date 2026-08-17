import { Extension } from '@tiptap/core'
import { Plugin, PluginKey } from '@tiptap/pm/state'
import { Decoration, DecorationSet } from '@tiptap/pm/view'

export const imageDropKey = new PluginKey('imageDrop')

/**
 * The image files out of a DataTransfer/Clipboard file list.
 * @param {*} list FileList or array-like
 * @returns {File[]}
 */
export function imageFilesFrom(list) {
  return Array.from(list || []).filter(
    (f) => f && typeof f.type === 'string' && f.type.startsWith('image/'),
  )
}

/**
 * Position of a live upload placeholder, or null once it is gone.
 *
 * The placeholder is a decoration, so it rides along with the document: text
 * typed above the drop point while the upload is in flight moves it, and the
 * image still lands where the user dropped it. Remembering a raw offset instead
 * is the classic way to have an image appear three paragraphs off.
 *
 * @param {object} state editor state
 * @param {object} id placeholder identity (an object used as a token)
 * @returns {number|null}
 */
export function findPlaceholder(state, id) {
  const found = imageDropKey.getState(state)?.find(null, null, (spec) => spec.id === id)
  return found?.length ? found[0].from : null
}

function placeholderPlugin() {
  return new Plugin({
    key: imageDropKey,
    state: {
      init: () => DecorationSet.empty,
      apply(tr, set) {
        set = set.map(tr.mapping, tr.doc)
        const action = tr.getMeta(imageDropKey)
        if (action?.add) {
          const el = document.createElement('span')
          el.className = 'doc-upload-placeholder'
          el.textContent = action.add.label || 'Загрузка изображения…'
          set = set.add(tr.doc, [
            Decoration.widget(action.add.pos, el, {
              id: action.add.id,
              side: action.add.side || 0,
            }),
          ])
        }
        if (action?.remove) {
          set = set.remove(set.find(null, null, (spec) => spec.id === action.remove.id))
        }
        return set
      },
    },
    props: {
      decorations: (state) => imageDropKey.getState(state),
    },
  })
}

/**
 * Uploads dropped/pasted files and inserts each as an image node.
 *
 * Placeholders for the whole batch go in first, then the uploads run one after
 * another. Both halves matter: reserving the spots up front is what keeps a
 * multi-file drop in the order the user dropped it, and serialising the uploads
 * keeps a drop of twenty photos from opening twenty parallel requests.
 *
 * @param {object} view ProseMirror editor view
 * @param {File[]} files image files
 * @param {number} pos insertion position
 * @param {object} options extension options ({ upload, onError })
 */
export async function uploadImagesAt(view, files, pos, options) {
  const jobs = files.map((file, i) => {
    const id = {}
    view.dispatch(
      view.state.tr.setMeta(imageDropKey, { add: { id, pos, side: i, label: file.name } }),
    )
    return { file, id }
  })

  for (const job of jobs) {
    try {
      const url = await options.upload(job.file)
      const at = findPlaceholder(view.state, job.id)
      const tr = view.state.tr.setMeta(imageDropKey, { remove: { id: job.id } })
      if (url && at !== null) {
        tr.insert(at, view.state.schema.nodes.image.create({ src: url, alt: job.file.name }))
      }
      view.dispatch(tr)
    } catch (err) {
      view.dispatch(view.state.tr.setMeta(imageDropKey, { remove: { id: job.id } }))
      options.onError?.(err)
    }
  }
}

/**
 * ImageDrop accepts images dropped onto the document from outside it, and the
 * same for paste — the "подтянуть картинку в документ" half of item 5 of #2718.
 *
 * The other half (dragging an image that is already in the document) needs no
 * code here: the Image node is draggable and ProseMirror moves it natively, so
 * the handlers below get out of the way for any drag that carries no files.
 */
export const ImageDrop = Extension.create({
  name: 'imageDrop',

  addOptions() {
    return { upload: null, onError: null }
  },

  addProseMirrorPlugins() {
    const options = this.options
    return [
      placeholderPlugin(),
      new Plugin({
        key: new PluginKey('imageDropHandlers'),
        props: {
          handleDrop(view, event) {
            const files = imageFilesFrom(event.dataTransfer?.files)
            // No files means ProseMirror's own drag — a block on the handle, or
            // an image moved inside the document. Returning false is what makes
            // those work; handling them here would break both.
            if (!files.length || !options.upload) return false
            event.preventDefault()
            const coords = view.posAtCoords({ left: event.clientX, top: event.clientY })
            uploadImagesAt(view, files, coords?.pos ?? view.state.selection.to, options)
            return true
          },
          handlePaste(view, event) {
            const files = imageFilesFrom(event.clipboardData?.files)
            if (!files.length || !options.upload) return false
            event.preventDefault()
            uploadImagesAt(view, files, view.state.selection.to, options)
            return true
          },
        },
      }),
    ]
  },
})
