// The per-turn action column body: while laying, the drawn-tile card + rotate; on your spend
// turn, the dice, bonus-move / portal rows, the selected-die action menu, patch-up / revive,
// and End turn. Shared by the classic top row (beside the sheet) and the compact left rail
// (inside your expanded block), so the two never drift. Pure presentational: all state and the
// lay/portal derivations are passed in from CrawlerPage.
import type { CrView, CrHero, CrLayOption, CrTile } from "./types";
import { DIRS, DIR_ARROW, tileKind } from "./data";
import { biomeTileDir } from "./logic";
import { Die } from "./Die";
import { TilePreview } from "./TilePreview";
import { CrawlerActions } from "./actions";

export function CrawlerActionPanel({
  v, me, myTurn, sel, setSel, send, setHlMon,
  layActive, layPreview, isPhaseLay, layRots, rotateLay, rotateLayBy,
  onPortalCenter, portalDests, portalNo, nameOf,
}: {
  v: CrView;
  me: CrHero | undefined;
  myTurn: boolean;
  sel: number | null;
  setSel: (n: number | null) => void;
  send: (msg: object) => void;
  setHlMon: (k: string | null) => void;
  layActive: boolean;
  layPreview: CrLayOption | null;
  isPhaseLay: boolean;
  layRots: number[];
  rotateLay: () => void;
  rotateLayBy: (delta: number) => void;
  onPortalCenter: boolean;
  portalDests: CrTile[];
  portalNo: (id: number) => number;
  nameOf: (s: number) => string;
}) {
  if (v.step === "lay") {
    return layActive && layPreview ? (
      <>
        <strong>{isPhaseLay ? "Lay phase · you drew" : "You drew"}: {tileKind(v.pending!.kind).label}</strong>
        <span className="muted-note">{tileKind(v.pending!.kind).blurb}</span>
        <div
          className="cr-lay-tile"
          draggable
          onDragStart={(e) => e.dataTransfer.setData("text/plain", "tile")}
          onWheel={(e) => { if (layRots.length > 1) { e.preventDefault(); rotateLayBy(e.deltaY > 0 ? 1 : -1); } }}
          title={isPhaseLay ? "Scroll to rotate me, then drop me on any glowing cell" : "Scroll to rotate me, then drop me on the glowing cell"}
        >
          <TilePreview kind={v.pending!.kind} edges={layPreview.edges} rot={layPreview.rotation} size={92} dir={biomeTileDir(v.biome)} />
        </div>
        <div className="cr-lay-ctl">
          {layRots.length > 1 && <button className="game-btn ghost" onClick={rotateLay}>⟳ Rotate (r / scroll)</button>}
          <span className="muted-note">
            {isPhaseLay
              ? "Rotate the tile (scroll over it, the ⟳ button, or r), then drag it onto a glowing cell, click one, or press Enter to lay it at the first spot. You move on your own turn."
              : "The spot is fixed (where you stepped): rotate the tile (scroll / ⟳ / r), then click the glowing cell or press Enter to step through."}
          </span>
        </div>
      </>
    ) : (
      <p className="muted-note">{nameOf(v.turn)} is laying a tile…</p>
    );
  }
  if (!(myTurn && me)) {
    return <p className="muted-note">Waiting for {nameOf(v.turn)}…</p>;
  }
  return (
    <>
      <div className="cr-dice">
        {/* shown sorted low -> high; the real index is kept for selecting/spending */}
        {v.dice.map((d, i) => ({ d, i })).sort((a, b) => a.d.value - b.d.value).map(({ d, i }) => (
          <Die
            key={i}
            value={d.value}
            spent={d.spent}
            selected={sel === i}
            usedFor={d.usedFor}
            bonus={!!d.bonus}
            disabled={d.spent}
            onClick={() => setSel(sel === i ? null : i)}
          />
        ))}
      </div>
      {v.bonusMoves > 0 && (
        <div className="cr-actions">
          <div className="cr-action-row">
            <span>Keep moving ({v.bonusMoves} left):</span>
            {DIRS.map((_, d) => (
              <button key={d} className="game-btn" onClick={() => send({ t: "step", dir: d })}>{DIR_ARROW[d]}</button>
            ))}
          </div>
        </div>
      )}
      {onPortalCenter && portalDests.length > 0 && (
        <div className="cr-actions">
          <div className="cr-action-row">
            <span>🌀 Portal to:</span>
            {portalDests.map((ti) => (
              <button key={ti.id} className="game-btn" onClick={() => { send({ t: "portal", tile: ti.id }); setSel(null); }}>→ Portal {portalNo(ti.id)}</button>
            ))}
          </div>
        </div>
      )}
      {sel != null && v.dice[sel] && !v.dice[sel].spent && (
        <CrawlerActions
          die={v.dice[sel]}
          me={me}
          view={v}
          onAct={(msg) => { send(msg); setSel(null); setHlMon(null); }}
          onReroll={() => { send({ t: "reroll", die: sel }); }}
          onHover={setHlMon}
        />
      )}
      {(() => {
        const hasDie = v.dice.some((d) => !d.spent);
        const downedAllies = v.heroes.filter((h) => h.downed && h.tile === me.tile && h.s !== me.s);
        const canHeal = me.courage >= 2 && me.health < me.maxHP;
        if (!canHeal && downedAllies.length === 0) return null;
        return (
          <div className="cr-actions">
            {canHeal && (
              <div className="cr-action-row">
                <button className="game-btn" title="Always available: trade 2 Courage for 1 Health" onClick={() => send({ t: "heal" })}>🩹 Patch up (2✊ → 1❤)</button>
              </div>
            )}
            {downedAllies.map((a) => (
              <div className="cr-action-row" key={a.s}>
                <span>⛑ {a.n} is down:</span>
                <button className="game-btn" disabled={!hasDie} title={hasDie ? "Spend a die to revive this ally (shortcut: v)" : "needs an unspent die"} onClick={() => { send({ t: "revive", ally: a.s }); setSel(null); }}>revive (1 die · v)</button>
              </div>
            ))}
          </div>
        );
      })()}
      <div className="cr-turn-actions">
        {v.cleared ? (
          <>
            <span className="cr-cleared-note">🏆 Boss defeated! Grab loot, then descend.</span>
            <button className="game-btn" onClick={() => { send({ t: "endTurn" }); setSel(null); }}>
              End turn{v.postBoss > 0 ? ` (${v.postBoss} left)` : ""}
            </button>
            <button className="game-btn cr-end-level" onClick={() => send({ t: "endLevel" })}>End level</button>
          </>
        ) : (
          <button className="game-btn" onClick={() => { send({ t: "endTurn" }); setSel(null); }}>End turn</button>
        )}
      </div>
    </>
  );
}
