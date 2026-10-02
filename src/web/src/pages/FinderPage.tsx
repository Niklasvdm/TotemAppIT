import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { listAnimals, similarByTraits } from "../api";
import { useFinder } from "../store";
import TraitPanel from "../components/TraitPanel";
import AnimalCard from "../components/AnimalCard";

export default function FinderPage({ emoji }: { emoji: Record<string, string> }) {
  const { t } = useTranslation();
  const { lang, query, include, exclude, setQuery } = useFinder();

  // Default behaviour is similarity: once any trait is included, rank by overlap;
  // with no traits picked, just list everything (optionally name-filtered).
  const similarMode = include.length > 0;

  const q = query.trim();

  const { data: shown = [], isLoading } = useQuery({
    queryKey: ["animals", lang, query, include, exclude],
    queryFn: async () => {
      if (!similarMode) return listAnimals({ lang, query, include, exclude });
      // Similarity mode: rank the whole profile by overlap.
      if (!q) return similarByTraits(include, exclude, lang);
      // A name search must span the WHOLE catalogue (a named animal may rank
      // past the default cap, or share no selected trait), so fetch name matches
      // separately and annotate each with its match score from a full ranking.
      const [named, ranked] = await Promise.all([
        listAnimals({ lang, query, include: [], exclude: [] }),
        similarByTraits(include, exclude, lang, 500),
      ]);
      const scoreBy = new Map(ranked.map((a) => [a.slug, a.score ?? 0]));
      return named
        .map((a) => ({ ...a, score: scoreBy.get(a.slug) ?? 0 }))
        .sort((x, y) => (y.score ?? 0) - (x.score ?? 0));
    },
  });

  return (
    <div className="layout">
      <TraitPanel />
      <section>
        <div className="searchbar">
          <span className="mag">🔍</span>
          <input
            value={query}
            placeholder={t("searchAnimal")}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
        <div className="countbar">
          <span className="n">{isLoading ? t("loading") : t("found", { n: shown.length })}</span>
        </div>
        <div className="grid">
          {shown.map((a) => (
            <AnimalCard key={a.slug} animal={a} emoji={emoji[a.slug] ?? "🐾"} />
          ))}
        </div>
      </section>
    </div>
  );
}
