import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

// Unit tests for the frontend's pure logic (the filter store, client-side
// movement prediction, game validation, quiz data) plus a few component tests
// (LocalSeats) rendered with react-dom/server. Node by default; files that
// touch the DOM / localStorage opt in with a `// @vitest-environment jsdom`
// pragma at the top. The React plugin lets `.test.tsx` files use JSX.
export default defineConfig({
  plugins: [react()],
  test: {
    environment: "node",
    include: ["src/**/*.test.{ts,tsx}"],
  },
});
