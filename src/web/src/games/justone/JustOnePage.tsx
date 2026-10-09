import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { createGameRoom } from "../../api";
import { CODE_LEN, NICK_MAX, validNick } from "../validate";
import { useRoom } from "../useRoom";
import { RoomBar } from "../RoomBar";

interface JoRoster {
  s: number;
  n: string;
  wrote: boolean;
  gone?: boolean;
}
interface JoView {
  t: "state";
  ph: "lobby" | "clue" | "guess" | "result" | "over";
  guesser: number;
  round: number;
  rounds: number;
  score: number;
  submitted: number;
  writers: number;
  word: string | null; // hidden from the guesser until the result
  yourClue: string;
  clues: string[]; // surviving (non-duplicate) clues
  lastGuess: string;
  correct: boolean;
  you: number;
  roster: JoRoster[];
}

export default function JustOnePage() {
  const { t } = useTranslation();
  const [params] = useSearchParams();
  const [name, setName] = useState(() => localStorage.getItem("totem-game-name") ?? "");
  const [code, setCode] = useState(() => (params.get("code") ?? "").toUpperCase());
  const [session, setSession] = useState<{ code: string; name: string } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  if (session) {
    return (
      <main className="game-page">
        <JustOneRoom code={session.code} name={session.name} onLeave={() => setSession(null)} />
      </main>
    );
  }

  const begin = (roomCode: string) => {
    const nick = name.trim();
    localStorage.setItem("totem-game-name", nick);
    setSession({ code: roomCode, name: nick });
  };
  const guard = () => {
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
      begin((await createGameRoom("just-one")).code);
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
        <h2>💡 {t("gameJustOne")}</h2>
        <p className="muted-note">{t("gameJustOneBlurb")}</p>
        <h3>{t("gameYourName")}</h3>
        <input
          className="trait-search"
          value={name}
          maxLength={NICK_MAX}
          placeholder={t("gameNamePlaceholder")}
          onChange={(e) => setName(e.target.value)}
        />
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

function JustOneRoom({ code, name, onLeave }: { code: string; name: string; onLeave: () => void }) {
  const { t } = useTranslation();
  const g = useRoom<JoView>(code, name);
  const [clue, setClue] = useState("");
  const [guess, setGuess] = useState("");
  const v = g.view;
  const bar = <RoomBar code={code} title={t("gameJustOne")} infoSlug="justone" onLeave={onLeave} />;

  if (g.status === "closed") {
    return (
      <div className="game-room">
        {bar}
        <div className="panel game-notice">
          <h3>Disconnected</h3>
          <button className="game-btn" onClick={onLeave}>
            {t("gameBack")}
          </button>
        </div>
      </div>
    );
  }
  if (!v) {
    return (
      <div className="game-room">
        {bar}
        <p className="muted-note" style={{ padding: "1rem" }}>
          {g.status === "reconnecting" ? "Reconnecting…" : t("loading")}
        </p>
      </div>
    );
  }

  const isHost = g.you >= 0 && g.you === g.host;
  const amGuesser = v.you === v.guesser;
  const guesserName = v.roster.find((r) => r.s === v.guesser)?.n ?? "";

  if (v.ph === "lobby") {
    return (
      <div className="game-room jo-room">
        {bar}
        <div className="mind-lobby">
          <p className="muted-note">3–8 players. Everyone but the guesser writes a one-word clue; identical clues cancel.</p>
          <ul className="mind-players">
            {v.roster.map((r) => (
              <li key={r.s} className={r.gone ? "gone" : ""}>
                👤 {r.n}
                {r.s === v.you ? " (you)" : ""}
              </li>
            ))}
          </ul>
          {isHost ? (
            <button className="game-btn" disabled={v.roster.length < 3} onClick={() => g.send({ t: "start" })}>
              {v.roster.length < 3 ? "Need at least 3 players" : "Start game"}
            </button>
          ) : (
            <p className="muted-note">Waiting for the host to start…</p>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="game-room jo-room">
      {bar}
      <div className="mind-hud panel">
        <span>Round {v.round}/{v.rounds}</span>
        <span>Score {v.score}</span>
        <span>{amGuesser ? "You are guessing" : `${guesserName} guesses`}</span>
      </div>

      {/* the mystery word — everyone but the guesser sees it */}
      {v.word != null && (
        <div className="jo-word">
          <small>Mystery word</small>
          <strong>{v.word}</strong>
        </div>
      )}

      {v.ph === "clue" &&
        (amGuesser ? (
          <p className="muted-note wl-wait">
            The others are writing clues… ({v.submitted}/{v.writers})
          </p>
        ) : v.yourClue ? (
          <p className="muted-note wl-wait">
            Your clue: <strong>{v.yourClue}</strong> · waiting for the others ({v.submitted}/{v.writers})
          </p>
        ) : (
          <div className="mind-controls panel">
            <input
              className="trait-search"
              value={clue}
              maxLength={40}
              placeholder="One-word clue"
              onChange={(e) => setClue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && clue.trim() && !clue.trim().includes(" ")) {
                  g.send({ t: "clue", clue: clue.trim() });
                  setClue("");
                }
              }}
            />
            <button
              className="game-btn"
              disabled={!clue.trim() || clue.trim().includes(" ")}
              onClick={() => {
                g.send({ t: "clue", clue: clue.trim() });
                setClue("");
              }}
            >
              Submit clue
            </button>
          </div>
        ))}

      {(v.ph === "guess" || v.ph === "result" || v.ph === "over") && (
        <div className="jo-clues">
          {v.clues.length === 0 ? (
            <span className="muted-note">All clues cancelled out! 😬</span>
          ) : (
            v.clues.map((c, i) => (
              <span key={i} className="jo-clue">
                {c}
              </span>
            ))
          )}
        </div>
      )}

      {v.ph === "guess" &&
        (amGuesser ? (
          <div className="mind-controls panel">
            <input
              className="trait-search"
              value={guess}
              placeholder="Your guess"
              onChange={(e) => setGuess(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && guess.trim()) {
                  g.send({ t: "guess", guess: guess.trim() });
                  setGuess("");
                }
              }}
            />
            <button className="game-btn" disabled={!guess.trim()} onClick={() => { g.send({ t: "guess", guess: guess.trim() }); setGuess(""); }}>
              Guess
            </button>
            <button className="game-btn ghost" onClick={() => g.send({ t: "pass" })}>
              Pass
            </button>
          </div>
        ) : (
          <p className="muted-note wl-wait">{guesserName} is guessing…</p>
        ))}

      {v.ph === "result" && (
        <div className="mind-controls panel mind-over">
          <strong>
            {v.lastGuess ? `“${v.lastGuess}” — ` : "Passed — "}
            {v.correct ? "correct! ✅" : `the word was “${v.word}” ❌`}
          </strong>
          {isHost && (
            <button className="game-btn" onClick={() => g.send({ t: "next" })}>
              Next word
            </button>
          )}
        </div>
      )}

      {v.ph === "over" && (
        <div className="mind-controls panel mind-over">
          <strong>Final score: {v.score} / {v.rounds} 🎉</strong>
          {isHost && (
            <button className="game-btn" onClick={() => g.send({ t: "restart" })}>
              New game
            </button>
          )}
        </div>
      )}
    </div>
  );
}
