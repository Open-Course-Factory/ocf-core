<script setup lang="ts">
const props = defineProps({
  class: {
    type: String,
    default: '',
  },
});
</script>

<template>
  <div class="slidev-layout main-toc">
    <LayoutHeader />
    <StripHeader />
    <div class="my-auto">
      <slot />
    </div>
    <LogoFooter />
    <LayoutFooter />
  </div>
</template>

