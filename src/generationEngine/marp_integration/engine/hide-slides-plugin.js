const marpHideSlidesPlugin = (md) => {
  md.marpit.customDirectives.local.hide = (value) => ({
    hide: value === 'true',
  })

  md.core.ruler.after(
    'marpit_directives_apply',
    'marpit_hide_slides',
    (state) => {
      let withinSlide = false
      let isHideSlide = false
      let slideStartIdx = 0
      let slideEndIdx = 0

      for (let i = 0; i < state.tokens.length; i += 1) {
        const token = state.tokens[i]

        if (!withinSlide && token.meta?.marpitSlideElement === 1) {
          withinSlide = true
          isHideSlide = false
          slideStartIdx = i
        }
        if (withinSlide) {
          if (token.meta?.marpitSlideElement === -1) {
            withinSlide = false
            slideEndIdx = i

            if (isHideSlide) {
              state.tokens.splice(
                slideStartIdx,
                slideEndIdx - slideStartIdx
              )
              i = slideStartIdx
            }
          } else if (
            token.type === 'marpit_slide_open' &&
            token.meta?.marpitDirectives.hide
          ) {
            isHideSlide = true
          }
        }
      }
    }
  )
}

module.exports = marpHideSlidesPlugin