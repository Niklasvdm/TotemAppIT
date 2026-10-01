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

  const { data: animals = [], isLoading } = useQuery({
    queryKey: ["animals", lang, query, include, exclude],
    queryFn: () =>
      similarMode
        ? similarByTraits(include, exclude, lang)
        : listAnimals({ lang, query, include, exclude }),
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
          <span className="n">{isLoading ? t("loading") : t("found", { n: animals.length })}</span>
        </div>
        <div className="grid">
          {animals.map((a) => (
            <AnimalCard key={a.slug} animal={a} emoji={emoji[a.slug] ?? "🐾"} />
          ))}
        </div>
      </section>
    </div>
  );
}
