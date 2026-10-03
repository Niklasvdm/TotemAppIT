import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import type { Input } from "../types";
import { SLOT_COLORS } from "../types";
import { decayOffset, sample, useGame } from "./net";
import NetHud, { useStatsFlag } from "./NetHud";
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

// maxCatchUp caps how much simulated time one frame may make up. Without it, a
// backgrounded tab returns and fires hundreds of input ticks at once.
const maxCatchUp = 250;

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
  const {
    arena,
    roster,
    you,
    phase,
    winner,
    world,
    self,
    net,
    pace,
    tickMs,
    fuseTicks,
    tickInput,
    sendStart,
  } = g;
  const showStats = useStatsFlag();

  const wrapRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const palRef = useRef(readPalette());
  const tileRef = useRef(26);

  const held = useRef<Set<string>>(new Set());
  // bombTap latches a press so one shorter than a tick is not missed between
  // two samples — the mirror of the server's own latch.
  const bombTap = useRef(false);

  const press = (token: string) => {
    held.current.add(token);
    if (token === "bomb") bombTap.current = true;
  };
  const release = (token: string) => held.current.delete(token);

  useEffect(() => {
    const onDown = (e: KeyboardEvent) => {
      const token = KEYS[e.code];
      if (!token) return;
      // Suppress the browser's own action FIRST: holding a key fires repeat
      // events continuously, and returning early on those let every one of them
      // through to scroll the page out from under the board.
      e.preventDefault();
      if (e.repeat) return;
      press(token);
    };
    const onUp = (e: KeyboardEvent) => {
      const token = KEYS[e.code];
      if (token) release(token);
    };
    // Losing focus mid-press would otherwise leave the player walking forever.
    const clear = () => held.current.clear();

    window.addEventListener("keydown", onDown);
    window.addEventListener("keyup", onUp);
    window.addEventListener("blur", clear);
    return () => {
      window.removeEventListener("keydown", onDown);
      window.removeEventListener("keyup", onUp);
      window.removeEventListener("blur", clear);
    };
  }, []);

  // Size the board to the container, keeping tiles square and crisp on HiDPI.
  useEffect(() => {
    const wrap = wrapRef.current;
    const canvas = canvasRef.current;
    if (!wrap || !canvas || !arena) return;

    const resize = () => {
      const tile = Math.max(
        14,
        Math.floor(
          Math.min(
            wrap.clientWidth / arena.w,
            (window.innerHeight * 0.68) / arena.h,
          ),
        ),
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
    obs.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ["data-theme"],
    });
    return () => obs.disconnect();
  }, []);

  useEffect(() => {
    if (!arena) return;
    let raf = 0;
    let last = performance.now();
    let carry = 0;

    const sampleInput = (): Input => {
      const h = held.current;
      const bomb = h.has("bomb") || bombTap.current;
      bombTap.current = false;
      return {
        dx: (h.has("right") ? 1 : 0) - (h.has("left") ? 1 : 0),
        dy: (h.has("down") ? 1 : 0) - (h.has("up") ? 1 : 0),
        bomb,
      };
    };

    const frame = (now: number) => {
      raf = requestAnimationFrame(frame);
      net.current.frames++;

      // Input runs on a fixed step at the server's rate, not at whatever the
      // display refreshes at — prediction only matches if each local tick
      // corresponds to exactly one server tick.
      const dt = now - last;
      carry = Math.min(carry + dt, maxCatchUp);
      last = now;
      // Ease the drawn position onto the simulated one; see net.ts.
      decayOffset(self.current, dt);
      // pace.factor steers this interval so the server's input queue stays
      // shallow; see the clock-sync note in net.ts.
      const step = tickMs * pace.current.factor;
      while (carry >= step) {
        carry -= step;
        tickInput(sampleInput());
      }

      const ctx = canvasRef.current?.getContext("2d");
      if (!ctx) return;
      const snap = sample(world.current, now, tickMs);
      if (!snap) return;

      // Everyone else is interpolated a tick behind; the local player is drawn
      // from prediction, which is what removes the felt input delay.
      const me = self.current;
      const shown =
        me.active && snap.p
          ? {
              ...snap,
              p: snap.p.map((p) =>
                p.s === you ? { ...p, x: me.x + me.ox, y: me.y + me.oy } : p,
              ),
            }
          : snap;

      draw({
        ctx,
        arena,
        crates: world.current.crates,
        snap: shown,
        roster,
        emoji,
        you,
        pal: palRef.current,
        tile: tileRef.current,
        now,
        fuseTicks,
      });
    };

    raf = requestAnimationFrame(frame);
    return () => cancelAnimationFrame(raf);
  }, [
    arena,
    roster,
    emoji,
    you,
    tickMs,
    fuseTicks,
    world,
    self,
    net,
    pace,
    tickInput,
  ]);

  const isHost = you >= 0 && you === g.host;
  const mine = roster.find((r) => r.s === you);
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
              className={`game-score ${r.s === you ? "mine" : ""} ${r.gone ? "gone" : ""}`}
              title={r.gone ? t("gameSeatWaiting") : undefined}
              style={{ borderColor: SLOT_COLORS[r.s % SLOT_COLORS.length] }}
            >
              <span className="game-score-animal">{emoji[r.m] ?? "🐾"}</span>
              <span className="game-score-name">{r.n}</span>
              <span className="game-score-wins">{r.w}</span>
              {r.gone && <span className="game-score-gone">⏳</span>}
            </div>
          ))}
        </div>
        <button className="game-btn ghost" onClick={onLeave}>
          {t("gameLeave")}
        </button>
      </div>

      {showStats && (
        <NetHud
          net={net}
          pace={pace}
          hz={Math.round(1000 / tickMs)}
          predicting={self.current.active}
          serverBuild={g.serverBuild}
        />
      )}

      <div className="game-stage" ref={wrapRef}>
        <canvas ref={canvasRef} className="game-canvas" />

        {g.status === "reconnecting" && (
          <div className="game-overlay">
            <h2>{t("gameReconnecting")}</h2>
            <p className="muted-note">{t("gameSeatHeld", { n: g.retries })}</p>
          </div>
        )}

        {g.status === "closed" && (
          <div className="game-overlay">
            <h2>{t("gameDisconnected")}</h2>
            <button className="game-btn" onClick={onLeave}>
              {t("gameBack")}
            </button>
          </div>
        )}

        {g.status === "open" && phase === "lobby" && (
          <div className="game-overlay">
            <h2>{t("gameWaiting")}</h2>
            <p className="game-share">
              {t("gameShareCode")} <strong>{code}</strong>
            </p>
            <div className="game-lobby-list">
              {roster.map((r) => (
                <span
                  key={r.s}
                  style={{ color: SLOT_COLORS[r.s % SLOT_COLORS.length] }}
                >
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

        {g.status === "open" && phase === "over" && (
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
          {mine && (
            <>
              <span>{emoji[mine.m] ?? "🐾"}</span>
              <span>💣 {g.stats.bombs}</span>
              <span>🔥 {g.stats.power}</span>
            </>
          )}
        </div>
        <p className="muted-note game-help">{t("gameControls")}</p>
      </div>

      <div className="game-pad">
        <div className="game-dpad">
          {(
            [
              ["up", "▲", "u"],
              ["left", "◀", "l"],
              ["down", "▼", "d"],
              ["right", "▶", "r"],
            ] as const
          ).map(([token, glyph, cls]) => (
            <button
              key={token}
              className={`game-dkey ${cls}`}
              onPointerDown={(e) => {
                e.preventDefault();
                press(token);
              }}
              onPointerUp={() => release(token)}
              onPointerLeave={() => release(token)}
              onPointerCancel={() => release(token)}
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
            press("bomb");
          }}
          onPointerUp={() => release("bomb")}
          onPointerCancel={() => release("bomb")}
          aria-label="bomb"
        >
          💣
        </button>
      </div>
    </div>
  );
}
