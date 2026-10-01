import { create } from "zustand";
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
  clear: () => void;
}

const without = (xs: string[], k: string) => xs.filter((x) => x !== k);

export const useFinder = create<FinderState>((set) => ({
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
  clear: () => set({ query: "", include: [], exclude: [] }),
}));
