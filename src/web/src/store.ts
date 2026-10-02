import { create } from "zustand";
import { persist } from "zustand/middleware";
import type { Lang } from "./api";

interface FinderState {
  lang: Lang;
  query: string;
  include: string[];
  exclude: string[];
  setLang: (l: Lang) => void;
  setQuery: (q: string) => void;
  addInclude: (key: string) => void;
  addExclude: (key: string) => void;
  removeTrait: (key: string) => void;
  applyProfile: (include: string[], exclude: string[]) => void; // quiz result → finder filters
  clearTraits: () => void; // remove all included + excluded traits
  clear: () => void;
}

// Keys may be a single trait ("rustig", from the quiz) or a synonym group
// ("stil|rustig", from a chip). Treat two entries as the same trait when they
// share ANY member, so a trait can't sit in include and exclude at once.
const members = (k: string) => k.split("|");
const overlaps = (a: string, b: string) => {
  const bs = new Set(members(b));
  return members(a).some((x) => bs.has(x));
};
const without = (xs: string[], k: string) => xs.filter((x) => !overlaps(x, k));

// Persisted to localStorage so a returning visitor keeps their last language and
// selected traits (lang/include/exclude only — see partialize).
export const useFinder = create<FinderState>()(
  persist(
    (set) => ({
      lang: "en",
      query: "",
      include: [],
      exclude: [],
      setLang: (lang) => set({ lang }),
      setQuery: (query) => set({ query }),
      addInclude: (key) =>
        set((s) => ({ include: [...without(s.include, key), key], exclude: without(s.exclude, key) })),
      addExclude: (key) =>
        set((s) => ({ exclude: [...without(s.exclude, key), key], include: without(s.include, key) })),
      removeTrait: (key) =>
        set((s) => ({ include: without(s.include, key), exclude: without(s.exclude, key) })),
      applyProfile: (include, exclude) => {
        const inc = [...new Set(include)];
        const exc = [...new Set(exclude)].filter((k) => !inc.some((i) => overlaps(i, k))); // include wins on conflict
        set({ include: inc, exclude: exc, query: "" });
      },
      clearTraits: () => set({ include: [], exclude: [] }),
      clear: () => set({ query: "", include: [], exclude: [] }),
    }),
    {
      name: "totem-finder",
      partialize: (s) => ({ lang: s.lang, include: s.include, exclude: s.exclude }),
    },
  ),
);
