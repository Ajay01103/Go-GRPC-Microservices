import { defineConfig } from "vite-plus"

export default defineConfig({
  lint: { options: { typeAware: true, typeCheck: true } },
  fmt: {
    semi: false,
    bracketSpacing: true,
    arrowParens: "always",
    bracketSameLine: true,
    insertFinalNewline: true,
    singleAttributePerLine: true,
  },
})
