// Typed client for the totemd API. All calls go through /api (Vite proxies to
// :8683 in dev; same-origin once the SPA is embedded in the binary).

export type Lang = "it" | "en" | "nl";

export interface Animal {
  slug: string;
  name: string;
  description: string;
  traits: string[];
  score?: number; // Jaccard 0..1, present only on similarity results
}

export interface AnimalImage {
  url: string;
  author?: string;
  license?: string;
  source?: string;
}

export interface AnimalDetail {
  slug: string;
  name: string;
  nameNl: string;
  altNames?: string;
  description: string;
  traits: string[];
  sourceUrl?: string;
  image?: AnimalImage;
}

export interface Trait {
  key: string;
  label: string;
  count: number;
}

export interface Filter {
  lang: Lang;
  query: string;
  include: string[];
  exclude: string[];
}

async function getJSON<T>(url: string): Promise<T> {
  const res = await fetch(url);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json() as Promise<T>;
}

export function listAnimals(f: Filter): Promise<Animal[]> {
  const p = new URLSearchParams({ lang: f.lang });
  if (f.query.trim()) p.set("q", f.query.trim());
  if (f.include.length) p.set("include", f.include.join(","));
  if (f.exclude.length) p.set("exclude", f.exclude.join(","));
  return getJSON<Animal[]>(`/api/v1/animals?${p}`);
}

export function getAnimal(slug: string, lang: Lang): Promise<AnimalDetail> {
  return getJSON<AnimalDetail>(`/api/v1/animals/${slug}?lang=${lang}`);
}

export function getSimilar(slug: string, lang: Lang, limit = 8): Promise<Animal[]> {
  return getJSON<Animal[]>(`/api/v1/animals/${slug}/similar?lang=${lang}&limit=${limit}`);
}

// Similarity mode: rank animals by overlap with the selected trait profile.
export function similarByTraits(include: string[], exclude: string[], lang: Lang): Promise<Animal[]> {
  const p = new URLSearchParams({ lang });
  if (include.length) p.set("include", include.join(","));
  if (exclude.length) p.set("exclude", exclude.join(","));
  return getJSON<Animal[]>(`/api/v1/similar?${p}`);
}

export function listTraits(lang: Lang): Promise<Trait[]> {
  return getJSON<Trait[]>(`/api/v1/traits?lang=${lang}`);
}

export function getEmojiMap(): Promise<Record<string, string>> {
  return getJSON<Record<string, string>>(`/api/v1/emoji`);
}
