import { describe, expect, it } from "vitest";
import { NICK_MAX, validNick } from "./validate";

// validNick mirrors cleanNick in internal/api/games.go — the server is the
// authority, this only saves a round trip, so the two must agree.
describe("validNick", () => {
  it("accepts ordinary names, including digits, accents and allowed punctuation", () => {
    for (const n of ["Niklas", "Niklas2", "José", "Anne-Marie", "O'Brien", "J.R.", "a"]) {
      expect(validNick(n)).toBe(true);
    }
  });

  it("trims and still accepts", () => {
    expect(validNick("  Fox  ")).toBe(true);
  });

  it("rejects empty or whitespace-only", () => {
    for (const n of ["", "   ", "\t"]) expect(validNick(n)).toBe(false);
  });

  it("accepts exactly NICK_MAX runes but not one more", () => {
    expect(validNick("x".repeat(NICK_MAX))).toBe(true);
    expect(validNick("x".repeat(NICK_MAX + 1))).toBe(false);
  });

  it("rejects angle brackets, control chars and emoji", () => {
    for (const n of ["<script>", "a<b", "a>b", "a\u0000b", "🦫"]) {
      expect(validNick(n)).toBe(false);
    }
  });
});
