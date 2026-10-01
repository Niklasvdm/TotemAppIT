import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQuery } from "@tanstack/react-query";
import { listTraits } from "../api";
import { useFinder } from "../store";

export default function TraitPanel() {
  const { t } = useTranslation();
  const { lang, include, exclude, addInclude, addExclude, removeTrait } = useFinder();
  const [q, setQ] = useState("");

  const { data: traits = [] } = useQuery({
    queryKey: ["traits", lang],
    queryFn: () => listTraits(lang),
  });

  const labelOf = (key: string) => traits.find((x) => x.key === key)?.label ?? key;

  const available = traits.filter(
    (tr) =>
      tr.label.toLowerCase().includes(q.toLowerCase()) &&
      !include.includes(tr.key) &&
      !exclude.includes(tr.key),
  );

  return (
    <aside className="sidebar">
      <div className="panel">
        <h3>{t("included")}</h3>
        <div className="pill-row">
          {include.length === 0 && <span className="muted-note">{t("none")}</span>}
          {include.map((k) => (
            <span key={k} className="chip inc" onClick={() => removeTrait(k)}>
              {labelOf(k)} <b className="x">✕</b>
            </span>
          ))}
        </div>
        <h3 style={{ marginTop: "1rem" }}>{t("excluded")}</h3>
        <div className="pill-row">
          {exclude.length === 0 && <span className="muted-note">{t("none")}</span>}
          {exclude.map((k) => (
            <span key={k} className="chip exc" onClick={() => removeTrait(k)}>
              {labelOf(k)} <b className="x">✕</b>
            </span>
          ))}
        </div>
      </div>

      <div className="panel">
        <h3>{t("traits")}</h3>
        <input
          className="trait-search"
          placeholder={t("searchTrait")}
          value={q}
          onChange={(e) => setQ(e.target.value)}
        />
        <div className="chips">
          {available.length === 0 && <span className="muted-note">{t("noResults")}</span>}
          {available.slice(0, 40).map((tr) => (
            <span key={tr.key} className="chip has-actions">
              {tr.label}
              <button className="add incb" title="include" onClick={() => addInclude(tr.key)}>
                ✓
              </button>
              <button className="add excb" title="exclude" onClick={() => addExclude(tr.key)}>
                ✗
              </button>
            </span>
          ))}
        </div>
      </div>
    </aside>
  );
}
