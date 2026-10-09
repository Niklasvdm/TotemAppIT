import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Link, useSearchParams } from "react-router-dom";
import { createGameRoom } from "../../api";
import { CODE_LEN, NICK_MAX, validNick } from "../validate";
import { useRoom } from "../useRoom";
import { RoomBar } from "../RoomBar";

interface DcHist {
  team: number;
  code: number[];
  clues: string[];
}
interface DcTeam {
  intercepts: number;
  miscomms: number;
  members: number;
}
interface DcRoster {
  s: number;
  n: string;
  team: number;
  gone?: boolean;
}
interface DcView {
  t: "state";
  ph: "lobby" | "clue" | "guess" | "reveal" | "over";
  round: number;
  rounds: number;
  you: number;
  team: number;
  activeTeam: number;
  encryptor: number;
  yourWords: string[] | null;
  code: number[] | null;
  clues: string[];
  decodeGuess: number[] | null;
  interceptGuess: number[] | null;
  decodeDone: boolean;
  interceptDone: boolean;
  history: DcHist[];
  teams: DcTeam[];
  winner: number;
  roster: DcRoster[];
}

export default function DecryptoPage() {
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
        <DecryptoRoom code={session.code} name={session.name} onLeave={() => setSession(null)} />
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
      begin((await createGameRoom("decrypto")).code);
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
        <h2>🔐 {t("gameDecrypto")}</h2>
        <p className="muted-note">{t("gameDecryptoBlurb")}</p>
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

// A 3-digit code picker over digits 1–4, forcing all three distinct.
function CodePicker({ value, onChange }: { value: number[]; onChange: (v: number[]) => void }) {
  const set = (i: number, d: number) => {
    const next = [...value];
    next[i] = d;
    onChange(next);
  };
  return (
    <div className="dc-code-pick">
      {[0, 1, 2].map((i) => (
        <select key={i} className="trait-search" value={value[i] ?? 0} onChange={(e) => set(i, Number(e.target.value))}>
          <option value={0}>–</option>
          {[1, 2, 3, 4].map((d) => <option key={d} value={d}>{d}</option>)}
        </select>
      ))}
    </div>
  );
}
const distinctCode = (c: number[]) => c.length === 3 && new Set(c).size === 3 && c.every((d) => d >= 1 && d <= 4);

function DecryptoRoom({ code, name, onLeave }: { code: string; name: string; onLeave: () => void }) {
  const { t } = useTranslation();
  const g = useRoom<DcView>(code, name);
  const [clues, setClues] = useState(["", "", ""]);
  const [myGuess, setMyGuess] = useState([0, 0, 0]);
  const v = g.view;
  const bar = <RoomBar code={code} title={t("gameDecrypto")} infoSlug="decrypto" onLeave={onLeave} />;

  if (g.status === "closed") {
    return (
      <div className="game-room">
        {bar}
        <div className="panel game-notice">
          <h3>Disconnected</h3>
          <button className="game-btn" onClick={onLeave}>{t("gameBack")}</button>
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
  const amEncryptor = v.encryptor === v.you;
  const onActiveTeam = v.team === v.activeTeam;
  const teamName = (tm: number) => `Team ${tm === 0 ? "🔵" : "🔴"}`;

  if (v.ph === "lobby") {
    const byTeam = (tm: number) => v.roster.filter((r) => r.team === tm);
    return (
      <div className="game-room dc-room">
        {bar}
        <div className="mind-lobby">
          <p className="muted-note">Two teams of 2+. Each team shares four secret words. Intercept the enemy code twice to win.</p>
          <div className="dc-teams-lobby">
            {[0, 1].map((tm) => (
              <div key={tm} className="dc-team-col">
                <h4>{teamName(tm)}</h4>
                <ul className="mind-players">
                  {byTeam(tm).map((r) => <li key={r.s} className={r.gone ? "gone" : ""}>👤 {r.n}{r.s === v.you ? " (you)" : ""}</li>)}
                </ul>
              </div>
            ))}
          </div>
          {isHost ? (
            <button
              className="game-btn"
              disabled={byTeam(0).length < 2 || byTeam(1).length < 2}
              onClick={() => g.send({ t: "start" })}
            >
              {byTeam(0).length < 2 || byTeam(1).length < 2 ? "Need 2 players per team" : "Start game"}
            </button>
          ) : (
            <p className="muted-note">Waiting for the host to start…</p>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="game-room dc-room">
      {bar}
      <div className="mind-hud panel">
        <span>Round {v.round}/{v.rounds}</span>
        <span>🔵 {v.teams[0].intercepts}✓ / {v.teams[0].miscomms}✗</span>
        <span>🔴 {v.teams[1].intercepts}✓ / {v.teams[1].miscomms}✗</span>
      </div>
      <p className="muted-note dc-turn">{teamName(v.activeTeam)} is encrypting · you are on {teamName(v.team)}</p>

      {v.yourWords && (
        <div className="dc-words panel">
          <small className="muted-note">Your team's secret words</small>
          <ol className="dc-wordlist">
            {v.yourWords.map((w, i) => <li key={i}><b>{i + 1}</b> {w}</li>)}
          </ol>
        </div>
      )}

      {v.code && (
        <div className="dc-code-show">
          Secret code: {v.code.map((d, i) => <span key={i} className="dc-digit">{d}</span>)}
        </div>
      )}

      {v.ph === "clue" && (
        amEncryptor ? (
          <div className="mind-controls panel">
            <small className="muted-note">Give one clue per code digit (each points at your word in that slot).</small>
            {[0, 1, 2].map((i) => (
              <input
                key={i}
                className="trait-search"
                value={clues[i]}
                placeholder={`Clue for digit ${v.code ? v.code[i] : "?"}`}
                onChange={(e) => setClues(clues.map((c, j) => (j === i ? e.target.value : c)))}
              />
            ))}
            <button
              className="game-btn"
              disabled={clues.some((c) => !c.trim())}
              onClick={() => { g.send({ t: "clue", clues: clues.map((c) => c.trim()) }); setClues(["", "", ""]); }}
            >
              Send clues
            </button>
          </div>
        ) : (
          <p className="muted-note wl-wait">Waiting for {teamName(v.activeTeam)}'s encryptor to write clues…</p>
        )
      )}

      {(v.ph === "guess" || v.ph === "reveal") && (
        <div className="dc-clues panel">
          <small className="muted-note">This round's clues</small>
          <ol className="dc-cluelist">
            {v.clues.map((c, i) => <li key={i}>{c}</li>)}
          </ol>
        </div>
      )}

      {v.ph === "guess" && (() => {
        const already = onActiveTeam ? v.decodeDone : v.interceptDone;
        const iGuess = onActiveTeam && !amEncryptor; // decode
        const iIntercept = !onActiveTeam; // intercept
        if (amEncryptor) return <p className="muted-note wl-wait">Your clues are out — teams are guessing.</p>;
        if (already) return <p className="muted-note wl-wait">Guess locked in. Waiting for the other side…</p>;
        if (!iGuess && !iIntercept) return <p className="muted-note wl-wait">Waiting…</p>;
        return (
          <div className="mind-controls panel">
            <small className="muted-note">
              {iGuess ? "Decode your own team's code from the clues." : "Intercept — guess the enemy code!"}
            </small>
            <CodePicker value={myGuess} onChange={setMyGuess} />
            <button
              className="game-btn"
              disabled={!distinctCode(myGuess)}
              onClick={() => { g.send({ t: iGuess ? "decode" : "intercept", guess: myGuess }); setMyGuess([0, 0, 0]); }}
            >
              Lock in {myGuess.filter((d) => d).join("-")}
            </button>
          </div>
        );
      })()}

      {v.ph === "reveal" && (
        <div className="mind-controls panel mind-over">
          <strong>Code was {v.code?.join("-")}</strong>
          <div className="dc-outcome">
            <span>Decode: {v.decodeGuess?.join("-")} {eqArr(v.decodeGuess, v.code) ? "✅" : "❌ (miscommunication)"}</span>
            <span>Intercept: {v.interceptGuess?.join("-")} {eqArr(v.interceptGuess, v.code) ? "🎯 intercepted!" : "—"}</span>
          </div>
          {isHost && <button className="game-btn" onClick={() => g.send({ t: "next" })}>Next round</button>}
        </div>
      )}

      {v.ph === "over" && (
        <div className="mind-controls panel mind-over">
          <strong>{v.winner < 0 ? "It's a draw!" : `${teamName(v.winner)} wins! 🎉`}</strong>
          {isHost && <button className="game-btn" onClick={() => g.send({ t: "restart" })}>New game</button>}
        </div>
      )}

      {v.history.length > 0 && (
        <div className="dc-history panel">
          <small className="muted-note">Past clues (for interception)</small>
          {v.history.map((h, i) => (
            <div key={i} className="dc-hist-row">
              <b>{teamName(h.team)}</b>: {h.clues.map((c, j) => <span key={j}>{c}→{h.code[j]} </span>)}
            </div>
          ))}
        </div>
      )}
    </div>
  );
}

function eqArr(a: number[] | null | undefined, b: number[] | null | undefined): boolean {
  if (!a || !b || a.length !== b.length) return false;
  return a.every((x, i) => x === b[i]);
}
