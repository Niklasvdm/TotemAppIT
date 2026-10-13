// CrawlerDebug is the F3 overlay (parallels Bomberman's NetHud): the state that would
// otherwise clutter the board — tile kinds and coords, and every monster's full stat
// line — lives here instead.
import type { CrView } from "./types";
import { tileKind } from "./data";

export function CrawlerDebug({ v }: { v: CrView }) {
  return (
    <div className="nethud cr-debug">
      <div className="nethud-title">crawler debug · phase {v.ph}/{v.step} · turn {v.turn} · deck {v.deckLeft}{v.grace >= 0 ? ` · collapse ${v.grace}` : ""}</div>
      {v.tiles.map((t) => (
        <div key={t.id} className="nethud-row">
          <span>#{t.id} {tileKind(t.kind).label} ({t.x},{t.y}) rot{t.rot}</span>
          <span>{t.monsters.filter((m) => m.alive).map((m) => `${m.kind} A${m.armour}/H${m.health}/D${m.damage}`).join(", ") || "—"}</span>
        </div>
      ))}
      <div className="nethud-hint">F3 to hide</div>
    </div>
  );
}
