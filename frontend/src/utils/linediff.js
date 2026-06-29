// Minimal line-level diff for the conflict resolver: given a base text and a
// changed text, return the changed text's lines each flagged whether it differs
// from the base (an LCS keeps unchanged lines aligned). Used to tint the lines
// that diverged in GitLab's vs Tessera's version, so the conflict is visible.
export function diffLines(base, cur) {
  const a = String(base ?? '').split('\n')
  const b = String(cur ?? '').split('\n')
  const n = a.length
  const m = b.length
  // LCS length table (suffix DP).
  const dp = Array.from({ length: n + 1 }, () => new Array(m + 1).fill(0))
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1])
    }
  }
  const out = []
  let i = 0
  let j = 0
  while (j < m) {
    if (i < n && a[i] === b[j]) {
      out.push({ text: b[j], changed: false }) // common line
      i++
      j++
    } else if (i < n && dp[i + 1][j] >= dp[i][j + 1]) {
      i++ // line only in base → skip (it isn't in `cur`)
    } else {
      out.push({ text: b[j], changed: true }) // line added/changed in `cur`
      j++
    }
  }
  return out
}
