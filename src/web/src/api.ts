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
  nameIt?: string;
  nameEn?: string;
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
  return getJSON<AnimalDetail>(`/api/v1/animals/${encodeURIComponent(slug)}?lang=${lang}`);
}

export function getSimilar(slug: string, lang: Lang, limit = 8): Promise<Animal[]> {
  return getJSON<Animal[]>(`/api/v1/animals/${encodeURIComponent(slug)}/similar?lang=${lang}&limit=${limit}`);
}

// Similarity mode: rank animals by overlap with the selected trait profile.
export function similarByTraits(
  include: string[],
  exclude: string[],
  lang: Lang,
  limit?: number,
): Promise<Animal[]> {
  const p = new URLSearchParams({ lang });
  if (include.length) p.set("include", include.join(","));
  if (exclude.length) p.set("exclude", exclude.join(","));
  if (limit) p.set("limit", String(limit));
  return getJSON<Animal[]>(`/api/v1/similar?${p}`);
}

export function listTraits(lang: Lang): Promise<Trait[]> {
  return getJSON<Trait[]>(`/api/v1/traits?lang=${lang}`);
}

export function getEmojiMap(): Promise<Record<string, string>> {
  return getJSON<Record<string, string>>(`/api/v1/emoji`);
}

async function postJSON(url: string, body: unknown): Promise<void> {
  const res = await fetch(url, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  if (!res.ok) {
    let msg = `${res.status}`;
    try {
      const j = await res.json();
      if (j?.error) msg = j.error;
    } catch {
      /* non-JSON error */
    }
    throw new Error(msg);
  }
}

// Community feedback.
export type ReportReason = "incorrect" | "unknown" | "poor_description" | "other";

export function suggestAnimal(name: string, note: string): Promise<void> {
  return postJSON(`/api/v1/suggestions`, { name, note });
}

export function reportAnimal(slug: string, reason: ReportReason, note: string): Promise<void> {
  return postJSON(`/api/v1/animals/${encodeURIComponent(slug)}/reports`, { reason, note });
}

// Report a problem with a game (bug, rule, unclear, other). `game` is the
// backend game slug (e.g. "the-mind", "crawler"), validated server-side.
export type GameReportReason = "bug" | "rules" | "unclear" | "other";

export function reportGame(game: string, reason: GameReportReason, note: string): Promise<void> {
  return postJSON(`/api/v1/games/reports`, { game, reason, note });
}

// Games. Creating a room allocates a tick loop on the server, so this is
// rate-limited there; the returned code is what other players type to join.
export async function createGameRoom(game = "bomberman"): Promise<{ code: string }> {
  const q = game && game !== "bomberman" ? `?game=${encodeURIComponent(game)}` : "";
  const res = await fetch(`/api/v1/games/rooms${q}`, { method: "POST" });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json() as Promise<{ code: string }>;
}
