import { defineConfig } from 'vite'
import svgLoader from 'vite-svg-loader'
import UnoCSS from 'unocss/vite'

export default defineConfig({
  plugins: [
    svgLoader(),
    // UnoCSS({
    //   configFile: './theme/uno.config.ts',
    // }),
  ],
})