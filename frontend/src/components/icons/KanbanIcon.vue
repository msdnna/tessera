<script setup>
// Custom kanban glyph in the ionicons outline style (stroke=currentColor, 512
// canvas) — ionicons5 has no kanban icon and pulling @vicons/fluent for one glyph
// isn't worth it. Three rounded columns, each with a card at the top. Usable as
// an n-icon :component or directly.
</script>

<template>
  <svg
    viewBox="0 0 512 512"
    fill="none"
    stroke="currentColor"
    stroke-width="30"
    stroke-linejoin="round"
    xmlns="http://www.w3.org/2000/svg"
  >
    <rect x="72" y="104" width="104" height="304" rx="22" />
    <rect x="204" y="104" width="104" height="304" rx="22" />
    <rect x="336" y="104" width="104" height="304" rx="22" />
    <rect x="96" y="140" width="56" height="72" rx="12" />
    <rect x="228" y="140" width="56" height="120" rx="12" />
    <rect x="360" y="140" width="56" height="88" rx="12" />
  </svg>
</template>
