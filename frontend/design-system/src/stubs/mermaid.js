// Mermaid (with cytoscape/katex) is ~8 MB — most of the bundle — for a feature a
// design never needs. The design system aliases it to this stub: RichContent's
// `initialize()` call throws inside its own try/catch, so ```mermaid blocks simply
// stay as code, exactly as on a failed diagram load in the app.
export default {
  initialize() {
    throw new Error('mermaid is not bundled in the Tessera design system')
  },
}
