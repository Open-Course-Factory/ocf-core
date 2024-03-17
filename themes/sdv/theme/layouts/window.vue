<template>
  <div class="slidev-layout">
    <LayoutHeader />
    <StripHeader />
    <div
      class="top"
    >
      <slot />
    </div>
    <WindowConsole class="rounded-lg shadow-lg object-cover z-10">
      <slot name="window" />
    </WindowConsole>
    <div
      class="bottom"
    >
      <slot name="bottom" />
    </div>
    <LayoutFooter />
  </div>
</template>
