import { Link, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { getAnimal, getSimilar } from "../api";
import { useFinder } from "../store";
import AnimalCard from "../components/AnimalCard";
import Flag from "../components/Flag";
import ReportBox from "../components/ReportBox";

export default function DetailPage({ emoji }: { emoji: Record<string, string> }) {
  const { t } = useTranslation();
  const { slug = "" } = useParams();
  const { lang } = useFinder();

  const { data: animal, isLoading } = useQuery({
    queryKey: ["animal", slug, lang],
    queryFn: () => getAnimal(slug, lang),
  });
  const { data: similar = [] } = useQuery({
    queryKey: ["similar", slug, lang],
    queryFn: () => getSimilar(slug, lang),
  });

  if (isLoading || !animal) {
    return (
      <div className="layout" style={{ gridTemplateColumns: "1fr" }}>
        <div className="detail">
          <Link to="/" className="back">{t("back")}</Link>
          <p className="muted-note">{t("loading")}</p>
        </div>
      </div>
    );
  }

  const glyph = emoji[animal.slug] ?? "🐾";

  return (
    <div className="layout" style={{ gridTemplateColumns: "1fr" }}>
      <div className="detail">
        <Link to="/" className="back">{t("back")}</Link>
        <ReportBox slug={animal.slug} />

        <div className="hero">
          <div className="photo-frame">
            {animal.image ? (
              <img src={animal.image.url} alt={animal.name} />
            ) : (
              <div className="patch" style={{ width: 190, height: 190, fontSize: "5rem" }}>{glyph}</div>
            )}
            {animal.image && <span className="emoji-badge">{glyph}</span>}
          </div>
          <h2>{animal.name}</h2>
          <div className="names">
            {[
              { code: "it" as const, name: animal.nameIt },
              { code: "gb" as const, name: animal.nameEn },
              { code: "be" as const, name: animal.nameNl }, // Belgian flag for the Dutch name
            ]
              .filter((n) => n.name)
              .map((n) => (
                <span key={n.code} className="name-lang">
                  <Flag code={n.code} /> {n.name}
                </span>
              ))}
          </div>
          {animal.altNames && <div className="alt-names">{animal.altNames}</div>}
          {animal.image?.license && (
            <div className="credit">
              📷 {animal.image.author || "Wikimedia Commons"} · {animal.image.license}
            </div>
          )}
          <div className="trait-badges">
            {animal.traits.map((tr) => (
              <span key={tr}>{tr}</span>
            ))}
          </div>
        </div>

        <p className="desc">{animal.description}</p>

        {similar.length > 0 && (
          <>
            <div className="section-label">{t("similarTo")}</div>
            <div className="similar-list">
              {similar.map((s) => (
                <AnimalCard key={s.slug} animal={s} emoji={emoji[s.slug] ?? "🐾"} />
              ))}
            </div>
          </>
        )}
      </div>
    </div>
  );
}
