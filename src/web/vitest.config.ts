import { defineConfig } from "vitest/config";

// Unit tests for the frontend's pure logic (the filter store, client-side
// movement prediction, game validation, quiz data). Node by default; files that
// touch the DOM / localStorage opt in with a `// @vitest-environment jsdom`
// pragma at the top.
export default defineConfig({
  test: {
    environment: "node",
    include: ["src/**/*.test.ts"],
  },
});
