import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";

// The games index. One entry for now; the list is the seam where the next game
// slots in without touching the router.
const GAMES = [
  { slug: "bomberman", icon: "💣", titleKey: "gameBomberman", blurbKey: "gameBombermanBlurb" },
  { slug: "codenames", icon: "🕵️", titleKey: "gameCodenames", blurbKey: "gameCodenamesBlurb" },
  { slug: "themind", icon: "🧠", titleKey: "gameTheMind", blurbKey: "gameTheMindBlurb" },
  { slug: "thegame", icon: "🃏", titleKey: "gameTheGame", blurbKey: "gameTheGameBlurb" },
];

export default function GamesPage() {
  const { t } = useTranslation();

  return (
    <main className="game-page">
      <div className="panel game-setup">
        <h2>🎲 {t("gamesTitle")}</h2>
        <p className="muted-note">{t("gamesBlurb")}</p>

        <div className="game-list">
          {GAMES.map((g) => (
            <Link key={g.slug} className="game-card" to={`/games/${g.slug}`}>
              <span className="game-card-icon">{g.icon}</span>
              <span className="game-card-title">{t(g.titleKey)}</span>
              <small className="game-card-blurb">{t(g.blurbKey)}</small>
            </Link>
          ))}
        </div>
      </div>
    </main>
  );
}
