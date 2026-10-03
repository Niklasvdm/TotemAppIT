import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { useFinder } from "../store";
import Flag from "./Flag";
import type { Lang } from "../api";

const LANGS: { code: Lang; flag: "it" | "gb" | "nl" }[] = [
  { code: "it", flag: "it" },
  { code: "en", flag: "gb" },
  { code: "nl", flag: "nl" },
];

export default function Header({
  onQuiz,
  onAbout,
  onSuggest,
}: {
  onQuiz: () => void;
  onAbout: () => void;
  onSuggest: () => void;
}) {
  const { t, i18n } = useTranslation();
  const { lang, setLang } = useFinder();

  // Apply the persisted language to the UI chrome on load.
  useEffect(() => {
    i18n.changeLanguage(lang);
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Initialise from the <html data-theme> set in index.html (dark by default).
  const [dark, setDark] = useState(
    () => document.documentElement.getAttribute("data-theme") !== "light",
  );

  const pick = (l: Lang) => {
    setLang(l);
    i18n.changeLanguage(l);
  };

  const toggleTheme = () => {
    const next = !dark;
    setDark(next);
    document.documentElement.setAttribute("data-theme", next ? "dark" : "light");
  };

  return (
    <header>
      <Link className="brand" to="/" aria-label={t("brand")}>
        <div className="mark">🐾</div>
        <div>
          <h1>{t("brand")}</h1>
          <small>{t("tagline")}</small>
        </div>
      </Link>
      <div className="header-tools">
        <button className="quiz-cta" onClick={onQuiz}>✨ Which animal are you?</button>
        <div className="langs">
          {LANGS.map((l) => (
            <button key={l.code} className={l.code === lang ? "active" : ""} onClick={() => pick(l.code)}>
              <Flag code={l.flag} />
              {l.code.toUpperCase()}
            </button>
          ))}
        </div>
        <Link className="iconbtn" to="/games" title={t("gamesTitle")}>
          🎲
        </Link>
        <button className="iconbtn" onClick={onSuggest} title="Suggest an animal">
          ➕
        </button>
        <button className="iconbtn" onClick={onAbout} title="How totems work">
          ℹ️
        </button>
        <button className="iconbtn" onClick={toggleTheme} title="theme">
          {dark ? "☀️" : "🌙"}
        </button>
      </div>
    </header>
  );
}
