// Transient over-the-board banners: the drop die after a kill and the status die after a
// robot's sting. Both auto-hide after a couple of seconds.
import { useEffect, useState } from "react";
import type { CrView } from "./types";
import { STATUSES, specialLabel } from "./data";
import { BossPortrait } from "./sprites";

// useAutoHide returns true, then false after `ms`, so a transient banner removes itself.
// Give the component a key tied to the event so a fresh event remounts it and the timer
// restarts (otherwise the one-shot timer never fires again).
export function useAutoHide(ms: number): boolean {
  const [shown, setShown] = useState(true);
  useEffect(() => {
    const t = setTimeout(() => setShown(false), ms);
    return () => clearTimeout(t);
  }, [ms]);
  return shown;
}

// CrawlerLootRoll is the visible drop die shown after a kill: the d100 result against the
// robot's drop chance, and whether an item fell.
export function CrawlerLootRoll({ roll }: { roll: NonNullable<CrView["lootRoll"]> }) {
  if (!useAutoHide(2000)) return null;
  return (
    <div className={`cr-lootroll${roll.hit ? " hit" : " miss"}`} key={`${roll.roll}-${roll.chance}`} role="status">
      <span className="cr-lootroll-die">🎲 {roll.roll}</span>
      <span className="cr-lootroll-vs">vs ≤{roll.chance}</span>
      <span className="cr-lootroll-result">
        {roll.hit ? `${roll.name || "Loot"} drops!` : "No drop"}
      </span>
    </div>
  );
}

// CrawlerLevelCard is the start-of-level title card: which biome/level, the boss that was drawn
// (sprite + name + its signature "twist" specials), and the total tile count, with a row of
// tiles "shuffling" beneath. Mounted keyed on the level (and boss/tile-count, so a retry
// re-triggers it). It does NOT auto-hide: the shuffle plays for a beat, then a ✕ / Begin button
// appears and the player dismisses it when ready.
export function CrawlerLevelCard({ v }: { v: CrView }) {
  const [done, setDone] = useState(false); // shuffle finished -> show the dismiss button
  const [closed, setClosed] = useState(false);
  useEffect(() => {
    const t = setTimeout(() => setDone(true), 2400);
    return () => clearTimeout(t);
  }, []);
  if (closed) return null;
  return (
    <div className="cr-levelcard" role="dialog" aria-label="level start">
      <div className="cr-levelcard-inner">
        {done && (
          <button className="cr-levelcard-x" onClick={() => setClosed(true)} title="Dismiss" aria-label="Dismiss">✕</button>
        )}
        <div className="cr-levelcard-level">Level {v.level}/6 · {v.biome}</div>
        <BossPortrait slug={v.bossSlug} size={92} />
        <div className="cr-levelcard-boss">👑 <strong>{v.bossName || "???"}</strong></div>
        {v.bossTwist.length > 0 && (
          <div className="cr-levelcard-twist">
            {v.bossTwist.map((s) => <span key={s} className="cr-levelcard-twist-item">⚡ {specialLabel(s)}</span>)}
          </div>
        )}
        <div className="cr-levelcard-tiles">🂠 {v.levelTiles} tiles to explore</div>
        {done ? (
          <button className="cr-levelcard-begin" onClick={() => setClosed(true)}>Begin ▶</button>
        ) : (
          <>
            <div className="cr-levelcard-shuffle" aria-hidden="true">
              {Array.from({ length: 7 }, (_, i) => (
                <span key={i} className="cr-shuffle-tile" style={{ animationDelay: `${i * 90}ms` }}>🂠</span>
              ))}
            </div>
            <div className="cr-levelcard-shuffle-label">shuffling the stack…</div>
          </>
        )}
      </div>
    </div>
  );
}

// CrawlerRoundWarning is the big start-of-round visibility banner. Mounted keyed on the turn so
// it fires once at the start of each moving round (after tile-laying): while tiles remain it
// warns when the stack is nearly out ("X tiles left! Boss near"); once the stack is empty it
// shows the collapse countdown ("N turns left before collapse"). Auto-hides after a few seconds.
export function CrawlerRoundWarning({ v }: { v: CrView }) {
  const shown = useAutoHide(6000);
  const [closed, setClosed] = useState(false);
  const showGrace = v.grace >= 0; // stack empty: collapse countdown
  const tilesLeft = v.deckLeft;
  const showTiles = !showGrace && tilesLeft > 0 && tilesLeft <= 10; // last 10 tiles
  if (closed || !shown || (!showGrace && !showTiles)) return null;
  return (
    <div className={`cr-round-warn${showGrace ? " collapse" : ""}`} role="status">
      <button className="cr-round-warn-x" onClick={() => setClosed(true)} title="Dismiss" aria-label="Dismiss">✕</button>
      {showGrace ? (
        <>
          <span className="cr-round-warn-ico">⚠</span>
          <span className="cr-round-warn-txt">{v.grace} turn{v.grace === 1 ? "" : "s"} left before the dungeon collapses!</span>
        </>
      ) : (
        <>
          <span className="cr-round-warn-ico">🂠</span>
          <span className="cr-round-warn-txt">{tilesLeft} tile{tilesLeft === 1 ? "" : "s"} left!{v.bossNear ? " Boss near." : ""}</span>
        </>
      )}
    </div>
  );
}

// CrawlerStatusRoll is the visible status die: when a robot's sting rolls for a status, it
// shows the d6 result against the needed face and whether the status landed.
export function CrawlerStatusRoll({ roll }: { roll: NonNullable<CrView["statusRoll"]> }) {
  const meta = STATUSES[roll.status];
  if (!useAutoHide(2000)) return null;
  return (
    <div className={`cr-statusroll${roll.hit ? " hit" : " miss"}`} key={`${roll.roll}-${roll.status}`} role="status">
      <span className="cr-lootroll-die">🎲 {roll.roll}</span>
      <span className="cr-lootroll-vs">vs ≥{roll.need}</span>
      <span className="cr-lootroll-result">
        {roll.hit ? `${roll.target ?? "A hero"} ${meta?.icon ?? ""} ${meta?.label ?? roll.status}` : "no status"}
      </span>
    </div>
  );
}
