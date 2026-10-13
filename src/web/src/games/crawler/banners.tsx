// Transient over-the-board banners: the drop die after a kill and the status die after a
// robot's sting. Both auto-hide after a couple of seconds.
import { useEffect, useState } from "react";
import type { CrView } from "./types";
import { STATUSES } from "./data";

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
