import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { createGameRoom, listAnimals } from "../../api";
import { useFinder } from "../../store";
import { CODE_LEN, NICK_MAX, validNick } from "../validate";
import GameRoom from "./GameRoom";

interface Session {
  code: string;
  name: string;
  animal: string;
}

export default function BombermanPage({ emoji }: { emoji: Record<string, string> }) {
  const { t } = useTranslation();
  const { lang } = useFinder();
  const [params] = useSearchParams();

  const [name, setName] = useState(() => localStorage.getItem("totem-game-name") ?? "");
  const [animal, setAnimal] = useState(() => localStorage.getItem("totem-game-animal") ?? "");
  const [code, setCode] = useState(() => (params.get("code") ?? "").toUpperCase());
  const [search, setSearch] = useState("");
  const [session, setSession] = useState<Session | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const { data: animals = [] } = useQuery({
    queryKey: ["animals", lang, ""],
    queryFn: () => listAnimals({ lang, query: "", include: [], exclude: [] }),
  });

  const shown = useMemo(() => {
    const q = search.trim().toLowerCase();
    const list = q ? animals.filter((a) => a.name.toLowerCase().includes(q)) : animals;
    return list.slice(0, 48);
  }, [animals, search]);

  if (session) {
    return (
      <main className="game-page">
        <GameRoom
          code={session.code}
          name={session.name}
          animal={session.animal}
          emoji={emoji}
          onLeave={() => setSession(null)}
        />
      </main>
    );
  }

  const begin = (roomCode: string) => {
    const nick = name.trim();
    localStorage.setItem("totem-game-name", nick);
    localStorage.setItem("totem-game-animal", animal);
    setSession({ code: roomCode, name: nick, animal });
  };

  const guard = (): boolean => {
    if (!validNick(name)) {
      setError(t("gameNeedName"));
      return false;
    }
    setError(null);
    return true;
  };

  const create = async () => {
    if (!guard()) return;
    setBusy(true);
    try {
      begin((await createGameRoom()).code);
    } catch {
      setError(t("gameNoRoom"));
    } finally {
      setBusy(false);
    }
  };

  const join = () => {
    if (!guard()) return;
    const c = code.trim().toUpperCase();
    if (c.length !== CODE_LEN) {
      setError(t("gameBadCode", { n: CODE_LEN }));
      return;
    }
    begin(c);
  };

  return (
    <main className="game-page">
      <div className="panel game-setup">
        <Link className="game-crumb" to="/games">
          {t("gameAllGames")}
        </Link>
        <h2>💣 {t("gameBomberman")}</h2>
        <p className="muted-note">{t("gameBombermanBlurb")}</p>

        <h3>{t("gameYourName")}</h3>
        <input
          className="trait-search"
          value={name}
          maxLength={NICK_MAX}
          placeholder={t("gameNamePlaceholder")}
          onChange={(e) => setName(e.target.value)}
        />

        <h3>{t("gameYourTotem")}</h3>
        <input
          className="trait-search"
          value={search}
          placeholder={t("searchTrait")}
          onChange={(e) => setSearch(e.target.value)}
        />
        <div className="game-animals">
          {shown.map((a) => (
            <button
              key={a.slug}
              className={`game-animal ${a.slug === animal ? "active" : ""}`}
              onClick={() => setAnimal(a.slug)}
              title={a.name}
            >
              <span>{emoji[a.slug] ?? "🐾"}</span>
              <small>{a.name}</small>
            </button>
          ))}
        </div>

        {error && <p className="game-error">{error}</p>}

        <div className="game-actions">
          <button className="game-btn" disabled={busy} onClick={create}>
            {busy ? t("loading") : t("gameCreate")}
          </button>
          <div className="game-join">
            <input
              className="trait-search game-codeinput"
              value={code}
              maxLength={CODE_LEN}
              placeholder={t("gameCodePlaceholder")}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
              onKeyDown={(e) => e.key === "Enter" && join()}
            />
            <button className="game-btn ghost" onClick={join}>
              {t("gameJoin")}
            </button>
          </div>
        </div>
      </div>
    </main>
  );
}
