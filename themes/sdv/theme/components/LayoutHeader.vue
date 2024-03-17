<template>
  <header
    class="
      flex
      absolute
      w-full
      top-0
      left-0
      text-sm
    "
  >
  <span v-if="$slidev.themeConfigs.title">
    {{
      $slidev.themeConfigs.title
    }}
  </span>
  </header>
</template>
