import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useFinder } from "../store";
import type { Lang } from "../api";

const LANGS: { code: Lang; flag: string }[] = [
  { code: "it", flag: "🇮🇹" },
  { code: "en", flag: "🇬🇧" },
  { code: "nl", flag: "🇳🇱" },
];

export default function Header() {
  const { t, i18n } = useTranslation();
  const { lang, setLang } = useFinder();
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
      <div className="brand">
        <div className="mark">🐾</div>
        <div>
          <h1>{t("brand")}</h1>
          <small>{t("tagline")}</small>
        </div>
      </div>
      <div className="header-tools">
        <div className="langs">
          {LANGS.map((l) => (
            <button key={l.code} className={l.code === lang ? "active" : ""} onClick={() => pick(l.code)}>
              <span className="flag">{l.flag}</span>
              {l.code.toUpperCase()}
            </button>
          ))}
        </div>
        <button className="iconbtn" onClick={toggleTheme} title="theme">
          {dark ? "☀️" : "🌙"}
        </button>
      </div>
    </header>
  );
}
