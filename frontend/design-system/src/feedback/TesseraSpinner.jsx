/**
 * TesseraSpinner — the branded animated loader (tessera tiles assembling), used
 * for every loading state instead of a generic spinner. `variant="white"` sits on
 * a coloured backdrop.
 * @category Feedback
 */
import TesseraSpinnerVue from '@/components/TesseraSpinner.vue'
import { vueIsland } from '../runtime/island'

export const TesseraSpinner = vueIsland(TesseraSpinnerVue, {
  name: 'TesseraSpinner',
  display: 'inline-flex',
})
