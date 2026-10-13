// CrawlerDebug is the F3 overlay (parallels Bomberman's NetHud). It shows the on-board
// state (tile kinds/coords + monster stat lines) and, when the run was started in debug
// mode (v.debug present), the deck peek (what will spawn next), the run's internal state,
// and the per-band spawn odds. Connection stats come from the room transport via `conn`.
import type { CrView } from "./types";
import { tileKind } from "./data";

export function CrawlerDebug({
  v,
  conn,
}: {
  v: CrView;
  conn?: { status: string; retries: number; seats: number; you: number; host: number };
}) {
  const d = v.debug;
  return (
    <div className="nethud cr-debug">
      <div className="nethud-title">
        crawler debug · phase {v.ph}/{v.step} · turn {v.turn} · deck {v.deckLeft}
        {v.grace >= 0 ? ` · collapse ${v.grace}` : ""}
      </div>

      {conn && (
        <div className="nethud-row">
          <span>conn: {conn.status}{conn.retries > 0 ? ` (retry ${conn.retries})` : ""}</span>
          <span>seat {conn.you} · host {conn.host} · {conn.seats} connected</span>
        </div>
      )}

      {d && (
        <>
          <div className="nethud-row">
            <span>seed {String(d.seed)} · L{d.level} {d.biome}</span>
            <span>deck {d.deckPos}/{d.deckLen}{d.forcedBoss ? ` · forced ${d.forcedBoss}` : ""}</span>
          </div>
          {(d.defeated?.length ?? 0) > 0 && (
            <div className="nethud-row"><span>beaten bosses</span><span>{d.defeated!.join(", ")}</span></div>
          )}
          {d.odds.map((line, i) => (
            <div key={`odd-${i}`} className="nethud-row"><span>{line}</span></div>
          ))}
          <div className="nethud-title">upcoming deck (top first)</div>
          {(d.upcoming ?? []).length === 0 && <div className="nethud-row"><span>— stack empty —</span></div>}
          {(d.upcoming ?? []).map((u) => (
            <div key={`up-${u.idx}`} className="nethud-row">
              <span>{u.idx}. {tileKind(u.kind).label}{u.isBoss ? " ★BOSS" : ""}</span>
              <span>{(u.monsters ?? []).join(", ") || "—"}</span>
            </div>
          ))}
          <div className="nethud-title">placed tiles</div>
        </>
      )}

      {v.tiles.map((t) => (
        <div key={t.id} className="nethud-row">
          <span>#{t.id} {tileKind(t.kind).label} ({t.x},{t.y}) rot{t.rot}</span>
          <span>{t.monsters.filter((m) => m.alive).map((m) => `${m.kind} A${m.armour}/H${m.health}/D${m.damage}`).join(", ") || "—"}</span>
        </div>
      ))}
      <div className="nethud-hint">F3 to hide{d ? "" : " · start a run in debug (F3 in the lobby) for the deck peek"}</div>
    </div>
  );
}
