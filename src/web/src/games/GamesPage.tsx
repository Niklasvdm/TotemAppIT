import { useTranslation } from "react-i18next";
import { Link } from "react-router-dom";
import { InfoButton } from "./GameRules";

// The games index. One entry for now; the list is the seam where the next game
// slots in without touching the router.
const GAMES = [
  { slug: "bomberman", icon: "💣", titleKey: "gameBomberman", blurbKey: "gameBombermanBlurb" },
  { slug: "codenames", icon: "🕵️", titleKey: "gameCodenames", blurbKey: "gameCodenamesBlurb" },
  { slug: "themind", icon: "🧠", titleKey: "gameTheMind", blurbKey: "gameTheMindBlurb" },
  { slug: "thegame", icon: "🃏", titleKey: "gameTheGame", blurbKey: "gameTheGameBlurb" },
  { slug: "wavelength", icon: "📡", titleKey: "gameWavelength", blurbKey: "gameWavelengthBlurb" },
  { slug: "justone", icon: "💡", titleKey: "gameJustOne", blurbKey: "gameJustOneBlurb" },
  { slug: "loveletter", icon: "💌", titleKey: "gameLoveLetter", blurbKey: "gameLoveLetterBlurb" },
  { slug: "decrypto", icon: "🔐", titleKey: "gameDecrypto", blurbKey: "gameDecryptoBlurb" },
  { slug: "hanabi", icon: "🎆", titleKey: "gameHanabi", blurbKey: "gameHanabiBlurb" },
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
            <div key={g.slug} className="game-card-wrap">
              <Link className="game-card" to={`/games/${g.slug}`}>
                <span className="game-card-icon">{g.icon}</span>
                <span className="game-card-title">{t(g.titleKey)}</span>
                <small className="game-card-blurb">{t(g.blurbKey)}</small>
              </Link>
              <InfoButton slug={g.slug} className="game-card-info" />
            </div>
          ))}
        </div>
      </div>
    </main>
  );
}
