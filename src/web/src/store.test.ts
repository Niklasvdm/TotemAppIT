// @vitest-environment jsdom
import { beforeEach, describe, expect, it } from "vitest";
import { useFinder } from "./store";

// The pill logic: a trait key may be a single key ("rustig") or a synonym group
// ("stil|rustig"), and the same trait must never sit in include AND exclude at
// once. These exercise that cross-list bookkeeping.
const reset = () => useFinder.setState({ query: "", include: [], exclude: [], lang: "en" });

describe("finder store", () => {
  beforeEach(reset);

  it("addInclude adds a trait", () => {
    useFinder.getState().addInclude("snel");
    expect(useFinder.getState().include).toEqual(["snel"]);
  });

  it("addExclude pulls a conflicting trait out of include (by group overlap)", () => {
    const s = useFinder.getState();
    s.addInclude("stil|rustig");
    s.addExclude("rustig"); // shares a member with the included group
    const st = useFinder.getState();
    expect(st.include).toEqual([]);
    expect(st.exclude).toEqual(["rustig"]);
  });

  it("addInclude pulls a conflicting trait out of exclude", () => {
    const s = useFinder.getState();
    s.addExclude("snel");
    s.addInclude("snel");
    const st = useFinder.getState();
    expect(st.exclude).toEqual([]);
    expect(st.include).toEqual(["snel"]);
  });

  it("removeTrait clears a trait from both lists by group overlap", () => {
    const s = useFinder.getState();
    s.addInclude("stil|rustig");
    s.removeTrait("rustig");
    expect(useFinder.getState().include).toEqual([]);
  });

  it("applyProfile dedupes and lets include win a conflict", () => {
    useFinder.getState().applyProfile(["a", "a", "b"], ["b", "c"]);
    const st = useFinder.getState();
    expect(st.include).toEqual(["a", "b"]);
    expect(st.exclude).toEqual(["c"]); // "b" dropped: it conflicts with an included key
  });

  it("clearTraits empties both lists", () => {
    const s = useFinder.getState();
    s.addInclude("a");
    s.addExclude("b");
    s.clearTraits();
    const st = useFinder.getState();
    expect(st.include).toEqual([]);
    expect(st.exclude).toEqual([]);
  });
});
