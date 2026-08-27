export const DARK = {
  bg: '#16161a',
  surface: '#1e1e24',
  surfaceAlt: '#26262e',
  cardSurface: '#1e1e24',
  // Background of text inputs — matches Naive's Input.color so hand-rolled
  // fields (the boxed comment composer) sit on the same surface as the rest.
  inputBg: '#26262e',
  hover: '#2d2d36',
  border: '#33333d',
  text1: '#f0f0f3',
  text2: '#d6d6dd',
  text3: '#9a9aa6',
  placeholder: '#5c5c66',
}

export const LIGHT = {
  bg: '#f6f7f9',
  surface: '#ffffff',
  surfaceAlt: '#f3f4f6',
  cardSurface: '#ffffff',
  inputBg: '#ffffff',
  hover: '#eef0f3',
  border: '#e6e8ec',
  text1: '#1f2329',
  text2: '#4a5059',
  text3: '#868d96',
  placeholder: '#b8bdc4',
}

// Task priority palette (index = priority 0..4). Used by board cards / pickers.
export const PRIORITY_COLORS = [
  '#9aa0aa', // 0 none
  '#5b9bd5', // 1 low
  '#3aa675', // 2 normal
  '#e0a418', // 3 high
  '#e0533d', // 4 urgent
]

// The matching labels are NOT here: a colour is a token, a string is not. They
// live in utils/priority.js, produced from the catalog per call (#2799).
