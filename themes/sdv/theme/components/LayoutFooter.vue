<template>
  <footer
    class="
      flex
      absolute
      w-full
      bottom-0
      my-auto
      py-0.5
      px-4
      text-sm
    "
  >
    
    <div
      v-if="$slidev.nav.currentLayout === 'intro'"
      class="w-auto m-auto text-left"
    >
      <span
        v-if="$slidev.themeConfigs.title && $slidev.themeConfigs.version"
        class="px-2"
      >
      &copy; {{ new Date().getFullYear() }}  
      {{ " - "+$slidev.themeConfigs.footerTitle+" "+$slidev.themeConfigs.version+" - "+$slidev.themeConfigs.author+ " - "}}
      <a href="">{{ $slidev.themeConfigs.email}}</a>
    
    </span>
    </div>
    <div
      v-else
      class="w-auto m-auto text-left"
    >
      {{ $slidev.themeConfigs.author+" - "+$slidev.themeConfigs.footerTitle+" - "+$frontmatter.chapter }}
    </div>
  </footer>
  <SlideCurrentNo v-if="$slidev.nav.currentLayout !== 'intro'" class="absolute left-5 bottom-1 text-base" />
</template>
