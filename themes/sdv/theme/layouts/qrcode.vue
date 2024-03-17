<script setup lang="ts">
import { computed } from 'vue'
import QRious from 'qrious'

const props = defineProps({
  url: {
    type: String,
    default: '',
  },
})

const qrcode = computed(() => {
    return new QRious({
        value: props.url,
        size: 300
    }).toDataURL()
})
</script>

<template>
  <div class="slidev-layout qrcode text-center">
    <LayoutHeader />
    <StripHeader />
    <slot />
    <div class="container mx-auto">
      <img
        :src="qrcode"
        alt="QR Code"
        class="w-m inline-block"
      >
    </div>
    <LayoutFooter />
  </div>
</template>
