import { describe, expect, it } from "vitest";
import { quiz } from "./quiz";

// The quiz is hand-authored data that feeds trait keys straight into the finder
// filters, so a typo here silently breaks matching. These guard its shape.
describe("quiz data", () => {
  it("every question has a prompt and at least two options", () => {
    expect(quiz.length).toBeGreaterThan(0);
    for (const q of quiz) {
      expect(q.prompt.trim().length).toBeGreaterThan(0);
      expect(q.options.length).toBeGreaterThanOrEqual(2);
    }
  });

  it("every option carries at least one well-formed trait key", () => {
    for (const q of quiz) {
      for (const o of q.options) {
        const keys = [...(o.include ?? []), ...(o.exclude ?? [])];
        expect(keys.length).toBeGreaterThan(0);
        for (const k of keys) expect(k).toMatch(/^[a-z]+$/); // Dutch trait keys: lowercase letters
      }
    }
  });

  it("no option both includes and excludes the same key", () => {
    for (const q of quiz) {
      for (const o of q.options) {
        const inc = new Set(o.include ?? []);
        for (const k of o.exclude ?? []) expect(inc.has(k)).toBe(false);
      }
    }
  });
});
