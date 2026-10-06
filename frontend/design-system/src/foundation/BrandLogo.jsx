/**
 * BrandLogo — the Tessera lockup: the lowercase "t" tile mark and/or the
 * "tessera" wordmark, painted with the accent gradient through CSS masks (follows
 * the accent scheme). Brand canon — never redraw or recolour it by hand.
 * @category Foundation
 */
import BrandLogoVue from '@/components/BrandLogo.vue'
import { vueIsland } from '../runtime/island'

export const BrandLogo = vueIsland(BrandLogoVue, { name: 'BrandLogo', display: 'inline-flex' })
