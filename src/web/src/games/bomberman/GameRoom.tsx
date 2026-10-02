import { useCallback, useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { SLOT_COLORS } from "../types";
import { sample, useGame } from "./net";
import { draw, readPalette } from "./render";

// Physical key codes, so the WASD cluster stays in the same place on an AZERTY
// keyboard (where those keys read ZQSD) as on QWERTY.
const KEYS: Record<string, string> = {
  ArrowUp: "up",
  ArrowDown: "down",
  ArrowLeft: "left",
  ArrowRight: "right",
  KeyW: "up",
  KeyS: "down",
  KeyA: "left",
  KeyD: "right",
  Space: "bomb",
  Enter: "bomb",
  KeyK: "bomb",
};

export default function GameRoom({
  code,
  name,
  animal,
  emoji,
  onLeave,
}: {
  code: string;
  name: string;
  animal: string;
  emoji: Record<string, string>;
  onLeave: () => void;
}) {
  const { t } = useTranslation();
  const g = useGame(code, name, animal);
  const { arena, roster, you, phase, winner, world, tickMs, sendInput, sendStart } = g;

  const wrapRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const palRef = useRef(readPalette());
  const tileRef = useRef(26);

  const held = useRef<Set<string>>(new Set());
  const sent = useRef({ dx: 0, dy: 0, bomb: false });

  // The server holds the last input until it changes, so only transitions need
  // to go on the wire.
  const flush = useCallback(() => {
    const h = held.current;
    const dx = (h.has("right") ? 1 : 0) - (h.has("left") ? 1 : 0);
    const dy = (h.has("down") ? 1 : 0) - (h.has("up") ? 1 : 0);
    const bomb = h.has("bomb");
    const last = sent.current;
    if (last.dx === dx && last.dy === dy && last.bomb === bomb) return;
    sent.current = { dx, dy, bomb };
    sendInput(dx, dy, bomb);
  }, [sendInput]);

  useEffect(() => {
    const onDown = (e: KeyboardEvent) => {
      const token = KEYS[e.code];
      if (!token || e.repeat) return;
      e.preventDefault(); // stop space and the arrows from scrolling the page
      held.current.add(token);
      flush();
    };
    const onUp = (e: KeyboardEvent) => {
      const token = KEYS[e.code];
      if (!token) return;
      held.current.delete(token);
      flush();
    };
    // Losing focus mid-press would otherwise leave the player walking forever.
    const release = () => {
      held.current.clear();
      flush();
    };

    window.addEventListener("keydown", onDown);
    window.addEventListener("keyup", onUp);
    window.addEventListener("blur", release);
    return () => {
      window.removeEventListener("keydown", onDown);
      window.removeEventListener("keyup", onUp);
      window.removeEventListener("blur", release);
    };
  }, [flush]);

  const touch = (token: string, down: boolean) => {
    if (down) held.current.add(token);
    else held.current.delete(token);
    flush();
  };

  // Size the board to the container, keeping tiles square and crisp on HiDPI.
  useEffect(() => {
    const wrap = wrapRef.current;
    const canvas = canvasRef.current;
    if (!wrap || !canvas || !arena) return;

    const resize = () => {
      const tile = Math.max(
        14,
        Math.floor(Math.min(wrap.clientWidth / arena.w, (window.innerHeight * 0.68) / arena.h)),
      );
      tileRef.current = tile;
      const dpr = window.devicePixelRatio || 1;
      canvas.width = arena.w * tile * dpr;
      canvas.height = arena.h * tile * dpr;
      canvas.style.width = `${arena.w * tile}px`;
      canvas.style.height = `${arena.h * tile}px`;
      canvas.getContext("2d")?.setTransform(dpr, 0, 0, dpr, 0, 0);
      palRef.current = readPalette();
    };

    resize();
    const ro = new ResizeObserver(resize);
    ro.observe(wrap);
    window.addEventListener("resize", resize);
    return () => {
      ro.disconnect();
      window.removeEventListener("resize", resize);
    };
  }, [arena]);

  // Follow the site's light/dark toggle.
  useEffect(() => {
    const obs = new MutationObserver(() => {
      palRef.current = readPalette();
    });
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ["data-theme"] });
    return () => obs.disconnect();
  }, []);

  useEffect(() => {
    if (!arena) return;
    let raf = requestAnimationFrame(function loop() {
      raf = requestAnimationFrame(loop);
      const ctx = canvasRef.current?.getContext("2d");
      if (!ctx) return;
      const now = performance.now();
      const snap = sample(world.current, now, tickMs);
      if (!snap) return;
      draw({
        ctx,
        arena,
        crates: world.current.crates,
        snap,
        roster,
        emoji,
        you,
        pal: palRef.current,
        tile: tileRef.current,
        now,
      });
    });
    return () => cancelAnimationFrame(raf);
  }, [arena, roster, emoji, you, tickMs, world]);

  const isHost = you >= 0 && you === g.host;
  const me = roster.find((r) => r.s === you);
  const winnerEntry = roster.find((r) => r.s === winner);

  if (g.error) {
    return (
      <div className="panel game-notice">
        <h3>{t("gameProblem")}</h3>
        <p className="muted-note">{g.error}</p>
        <button className="game-btn" onClick={onLeave}>
          {t("gameBack")}
        </button>
      </div>
    );
  }

  return (
    <div className="game-room">
      <div className="game-bar panel">
        <div className="game-code">
          <small>{t("gameRoomCode")}</small>
          <strong>{code}</strong>
        </div>
        <div className="game-scores">
          {roster.map((r) => (
            <div
              key={r.s}
              className={`game-score ${r.s === you ? "mine" : ""}`}
              style={{ borderColor: SLOT_COLORS[r.s % SLOT_COLORS.length] }}
            >
              <span className="game-score-animal">{emoji[r.m] ?? "🐾"}</span>
              <span className="game-score-name">{r.n}</span>
              <span className="game-score-wins">{r.w}</span>
            </div>
          ))}
        </div>
        <button className="game-btn ghost" onClick={onLeave}>
          {t("gameLeave")}
        </button>
      </div>

      <div className="game-stage" ref={wrapRef}>
        <canvas ref={canvasRef} className="game-canvas" />

        {g.status === "closed" && (
          <div className="game-overlay">
            <h2>{t("gameDisconnected")}</h2>
            <button className="game-btn" onClick={onLeave}>
              {t("gameBack")}
            </button>
          </div>
        )}

        {g.status !== "closed" && phase === "lobby" && (
          <div className="game-overlay">
            <h2>{t("gameWaiting")}</h2>
            <p className="game-share">
              {t("gameShareCode")} <strong>{code}</strong>
            </p>
            <div className="game-lobby-list">
              {roster.map((r) => (
                <span key={r.s} style={{ color: SLOT_COLORS[r.s % SLOT_COLORS.length] }}>
                  {emoji[r.m] ?? "🐾"} {r.n}
                </span>
              ))}
            </div>
            {isHost ? (
              <button className="game-btn" onClick={sendStart}>
                {t("gameStart")}
              </button>
            ) : (
              <p className="muted-note">{t("gameHostStarts")}</p>
            )}
          </div>
        )}

        {g.status !== "closed" && phase === "over" && (
          <div className="game-overlay">
            <h2>
              {winnerEntry
                ? `${emoji[winnerEntry.m] ?? "🐾"} ${t("gameWins", { name: winnerEntry.n })}`
                : t("gameDraw")}
            </h2>
            {isHost ? (
              <button className="game-btn" onClick={sendStart}>
                {t("gameNextRound")}
              </button>
            ) : (
              <p className="muted-note">{t("gameHostStarts")}</p>
            )}
          </div>
        )}
      </div>

      <div className="game-footer">
        <div className="game-stats">
          {me && (
            <>
              <span>{emoji[me.m] ?? "🐾"}</span>
              <span>💣 {g.stats.bombs}</span>
              <span>🔥 {g.stats.power}</span>
            </>
          )}
        </div>
        <p className="muted-note game-help">{t("gameControls")}</p>
      </div>

      <div className="game-pad">
        <div className="game-dpad">
          {([
            ["up", "▲", "u"],
            ["left", "◀", "l"],
            ["down", "▼", "d"],
            ["right", "▶", "r"],
          ] as const).map(([token, glyph, cls]) => (
            <button
              key={token}
              className={`game-dkey ${cls}`}
              onPointerDown={(e) => {
                e.preventDefault();
                touch(token, true);
              }}
              onPointerUp={() => touch(token, false)}
              onPointerLeave={() => touch(token, false)}
              onPointerCancel={() => touch(token, false)}
              aria-label={token}
            >
              {glyph}
            </button>
          ))}
        </div>
        <button
          className="game-bombkey"
          onPointerDown={(e) => {
            e.preventDefault();
            touch("bomb", true);
          }}
          onPointerUp={() => touch("bomb", false)}
          onPointerCancel={() => touch("bomb", false)}
          aria-label="bomb"
        >
          💣
        </button>
      </div>
    </div>
  );
}
