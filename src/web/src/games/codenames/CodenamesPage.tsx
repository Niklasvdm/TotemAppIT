import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { createGameRoom } from "../../api";
import { CODE_LEN, NICK_MAX, validNick } from "../validate";
import { useRoom } from "../useRoom";
import { RoomBar } from "../RoomBar";

// Mirrors cnView in internal/game/codenames.go. `key` is present only when the
// server decides this seat is a spymaster — that is the private view.
interface CnRoster {
  s: number;
  n: string;
  team: number; // 0 red, 1 blue
  spymaster: boolean;
  gone?: boolean;
}
interface CnClue {
  word: string;
  count: number;
}
interface CnView {
  t: "state";
  ph: "lobby" | "clue" | "guess" | "over";
  words: string[];
  rev: boolean[];
  shown: string[]; // revealed colours, visible to all ("" while hidden)
  key: string[] | null; // full key, spymasters only
  turn: number;
  clue: CnClue | null;
  guesses: number;
  redLeft: number;
  blueLeft: number;
  winner: number; // -1 none
  you: CnRoster;
  roster: CnRoster[];
}

const teamName = (t: number) => (t === 1 ? "Blue" : "Red");

export default function CodenamesPage() {
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
        <CodenamesRoom code={session.code} name={session.name} onLeave={() => setSession(null)} />
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
      begin((await createGameRoom("codenames")).code);
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
        <h2>🕵️ {t("gameCodenames")}</h2>
        <p className="muted-note">{t("gameCodenamesBlurb")}</p>

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

function CodenamesRoom({
  code,
  name,
  onLeave,
}: {
  code: string;
  name: string;
  onLeave: () => void;
}) {
  const { t } = useTranslation();
  const g = useRoom<CnView>(code, name);
  const v = g.view;

  const bar = <RoomBar code={code} title={t("gameCodenames")} infoSlug="codenames" onLeave={onLeave} />;

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
  const myTurn = v.you.team === v.turn;

  return (
    <div className="game-room cn-room">
      {bar}

      {v.ph === "lobby" ? (
        <Lobby v={v} isHost={isHost} send={g.send} />
      ) : (
        <>
          <Scoreboard v={v} />
          <Board v={v} onGuess={(i) => g.send({ t: "guess", tile: i })} />
          <Controls v={v} myTurn={myTurn} isHost={isHost} send={g.send} />
        </>
      )}
    </div>
  );
}

function Lobby({
  v,
  isHost,
  send,
}: {
  v: CnView;
  isHost: boolean;
  send: (m: object) => void;
}) {
  const setup = (team: number, spymaster: boolean) => send({ t: "setup", team, spymaster });
  const hasSpy = [0, 1].map((tm) => v.roster.some((p) => p.team === tm && p.spymaster));
  const hasGuess = [0, 1].map((tm) => v.roster.some((p) => p.team === tm && !p.spymaster));
  const canStart = hasSpy[0] && hasSpy[1] && hasGuess[0] && hasGuess[1];

  return (
    <div className="cn-lobby">
      <div className="cn-teams">
        {[0, 1].map((tm) => (
          <div key={tm} className={`cn-team cn-team-${tm === 1 ? "blue" : "red"}`}>
            <h3>{teamName(tm)}</h3>
            <ul>
              {v.roster
                .filter((p) => p.team === tm)
                .map((p) => (
                  <li key={p.s} className={p.gone ? "gone" : ""}>
                    {p.spymaster ? "🕵️ " : "👤 "}
                    {p.n}
                    {p.s === v.you.s ? " (you)" : ""}
                  </li>
                ))}
            </ul>
            <div className="cn-join-buttons">
              <button className="game-btn ghost" onClick={() => setup(tm, false)}>
                Join as guesser
              </button>
              <button
                className="game-btn ghost"
                disabled={v.roster.some((p) => p.team === tm && p.spymaster && p.s !== v.you.s)}
                onClick={() => setup(tm, true)}
              >
                {v.roster.some((p) => p.team === tm && p.spymaster && p.s !== v.you.s)
                  ? "Spymaster taken"
                  : "Be spymaster"}
              </button>
            </div>
          </div>
        ))}
      </div>
      {isHost ? (
        <button className="game-btn cn-start" disabled={!canStart} onClick={() => send({ t: "start" })}>
          {canStart ? "Start game" : "Need a spymaster + guesser on each team"}
        </button>
      ) : (
        <p className="muted-note">Waiting for the host to start…</p>
      )}
    </div>
  );
}

function Scoreboard({ v }: { v: CnView }) {
  return (
    <div className="cn-score panel">
      <span className="cn-score-red">Red {v.redLeft}</span>
      <span className="cn-turn">
        {v.winner >= 0
          ? `${teamName(v.winner)} wins!`
          : v.ph === "clue"
            ? `${teamName(v.turn)} spymaster is thinking…`
            : v.clue
              ? `${teamName(v.turn)}: “${v.clue.word}” ${v.clue.count}  ·  ${v.guesses} left`
              : `${teamName(v.turn)} guessing`}
      </span>
      <span className="cn-score-blue">Blue {v.blueLeft}</span>
    </div>
  );
}

function Board({ v, onGuess }: { v: CnView; onGuess: (i: number) => void }) {
  const myTurn = v.you.team === v.turn;
  const canGuess = v.ph === "guess" && myTurn && !v.you.spymaster;
  return (
    <div className="cn-board">
      {v.words.map((word, i) => {
        const revealed = v.rev[i];
        const shownCol = revealed ? v.shown[i] : "";
        const spyCol = v.key ? v.key[i] : "";
        const classes = ["cn-tile"];
        if (revealed) classes.push("cn-revealed", `cn-${shownCol}`);
        else if (spyCol) classes.push(`cn-spy-${spyCol}`);
        const clickable = canGuess && !revealed;
        return (
          <button
            key={i}
            className={classes.join(" ") + (clickable ? " cn-clickable" : "")}
            disabled={!clickable}
            onClick={() => clickable && onGuess(i)}
          >
            {word}
          </button>
        );
      })}
    </div>
  );
}

function Controls({
  v,
  myTurn,
  isHost,
  send,
}: {
  v: CnView;
  myTurn: boolean;
  isHost: boolean;
  send: (m: object) => void;
}) {
  const [word, setWord] = useState("");
  const [count, setCount] = useState(1);

  if (v.ph === "over") {
    return (
      <div className="cn-controls panel">
        <strong>{teamName(v.winner)} wins!</strong>
        {isHost && (
          <button className="game-btn" onClick={() => send({ t: "restart" })}>
            New game
          </button>
        )}
      </div>
    );
  }

  if (v.ph === "clue") {
    if (myTurn && v.you.spymaster) {
      const give = () => {
        const w = word.trim();
        if (w && !w.includes(" ")) {
          send({ t: "clue", word: w, count });
          setWord("");
        }
      };
      return (
        <div className="cn-controls panel">
          <input
            className="trait-search"
            value={word}
            placeholder="One-word clue"
            onChange={(e) => setWord(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && give()}
          />
          <select value={count} onChange={(e) => setCount(Number(e.target.value))}>
            {[0, 1, 2, 3, 4, 5, 6, 7, 8, 9].map((n) => (
              <option key={n} value={n}>
                {n === 0 ? "∞" : n}
              </option>
            ))}
          </select>
          <button className="game-btn" onClick={give}>
            Give clue
          </button>
        </div>
      );
    }
    return <p className="muted-note cn-wait">Waiting for the {teamName(v.turn)} spymaster…</p>;
  }

  // guess phase
  if (myTurn && !v.you.spymaster) {
    return (
      <div className="cn-controls panel">
        <span className="muted-note">Tap a card to guess, or end your turn.</span>
        <button className="game-btn ghost" onClick={() => send({ t: "pass" })}>
          End turn
        </button>
      </div>
    );
  }
  return <p className="muted-note cn-wait">{teamName(v.turn)} is guessing…</p>;
}
