// The on-pawn touch overlay (mobile): everything the sidebar holds, floated around your hero
// so the side panel is not needed. The d-pad is centred ON the pawn; the dice read-out and the
// hearts/shields sit above it; active statuses hang to the left (hoverable); the action bars
// and items sit to the right; End turn is under the d-pad. The cluster re-anchors on the pawn
// on every move so it tracks the character. Shown only on your own spend turn.
import { useLayoutEffect, useRef, useState, type RefObject } from "react";
import type { CrView, CrHero, CrTile } from "./types";
import { heroStatuses, STATUSES } from "./data";
import { TotemSprite } from "./sprites";
import { CrawlerDiceRow } from "./dicerow";
import { CrawlerMovePad } from "./movepad";
import { CrawlerActionBars, CrawlerItemRow } from "./actionbars";

export function CrawlerOverlay({ v, me, showMove, onSend, boardRef, lucky, onArmLucky, onPickLucky, onReroll, carry, onPortalCenter, portalDests, portalNo, onPortal }: {
  v: CrView;
  me: CrHero;
  showMove: boolean; // the move-pad toggle
  onSend: (msg: object) => void;
  boardRef: RefObject<HTMLDivElement | null>; // the .cr-board-wrap, to anchor the cluster on the pawn
  lucky: boolean;
  onArmLucky: () => void;
  onPickLucky: (dieIndex: number) => void;
  onReroll: (dieIndex: number) => void;
  carry: number | null;
  onPortalCenter: boolean;
  portalDests: CrTile[];
  portalNo: (id: number) => number;
  onPortal: (tileId: number) => void;
}) {
  const carried = carry != null ? v.heroes.find((h) => h.s === carry) : undefined;
  const myTile = v.tiles.find((t) => t.id === me.tile);
  // Keep the cluster centred on YOUR pawn, re-measuring on move / the map's re-centre scroll /
  // resize. (Enemy attack ⚔ live ON the board, not here, so they scroll natively and show
  // whether or not this overlay is up.)
  const [pos, setPos] = useState<{ x: number; y: number } | null>(null);
  const rafRef = useRef(0);
  useLayoutEffect(() => {
    const wrap = boardRef.current;
    if (!wrap) return;
    const measure = () => {
      cancelAnimationFrame(rafRef.current);
      rafRef.current = requestAnimationFrame(() => {
        const pawn = wrap.querySelector(".cr-pawn.you") as HTMLElement | null;
        if (!pawn) { setPos(null); return; }
        const wr = wrap.getBoundingClientRect();
        const pr = pawn.getBoundingClientRect();
        setPos({ x: pr.left + pr.width / 2 - wr.left, y: pr.top + pr.height / 2 - wr.top });
      });
    };
    measure();
    const scroller = wrap.querySelector(".cr-map-scroll");
    scroller?.addEventListener("scroll", measure, { passive: true });
    window.addEventListener("resize", measure);
    return () => {
      cancelAnimationFrame(rafRef.current);
      scroller?.removeEventListener("scroll", measure);
      window.removeEventListener("resize", measure);
    };
  }, [boardRef, me.tile, me.square, v.tiles.length]);
  const anchorStyle = pos ? { left: pos.x, top: pos.y } : { left: "50%", top: "50%" };

  const statuses = heroStatuses(me);

  return (
    <div className="cr-overlay" aria-label="touch controls">
      <div className="cr-ov-anchor" style={anchorStyle}>
        {/* dice (or, while carrying, the carried ally's sprite) above the hero */}
        {carried ? (
          <div className="cr-ov-dice cr-ov-carry" title={`Carrying ${carried.n} — pick a direction (tap them again to stop)`}>
            <TotemSprite slug={carried.totem} size={30} />
          </div>
        ) : (
          <CrawlerDiceRow v={v} me={me} className="cr-ov-dice" lucky={lucky} onPickLucky={onPickLucky} onReroll={onReroll} />
        )}
        {/* portals: an arrow around the hero pointing toward each other portal (true angle) */}
        {onPortalCenter && myTile && portalDests.map((p) => {
          const ang = Math.atan2(p.y - myTile.y, p.x - myTile.x); // screen y is down, matching cell y
          const R = 5.2;
          return (
            <button
              key={p.id}
              className="cr-ov-portal"
              style={{ transform: `translate(-50%, -50%) translate(${Math.cos(ang) * R}rem, ${Math.sin(ang) * R}rem)` }}
              onClick={() => onPortal(p.id)}
              title={`Portal to ${portalNo(p.id)}`}
            >
              <span className="cr-ov-portal-arrow" style={{ transform: `rotate(${ang}rad)` }}>➤</span>
              <span className="cr-ov-portal-no">{portalNo(p.id)}</span>
            </button>
          );
        })}
        {/* a compact 4-heart gauge above the sprite (proportional; exact value in the title) */}
        {(() => {
          const HEARTS = 4;
          const filled = Math.max(0, Math.min(HEARTS, Math.round((me.health / me.maxHP) * HEARTS)));
          return (
            <div className="cr-ov-hp" title={`${me.health}/${me.maxHP} health${me.shields ? ` · ${me.shields} shields` : ""}`}>
              {Array.from({ length: HEARTS }, (_, i) => (
                <span key={i} className="cr-pip-heart">{i < filled ? "❤️" : "🤍"}</span>
              ))}
              {Array.from({ length: me.shields }, (_, i) => (
                <span key={`s${i}`} className="cr-pip-shield">🛡️</span>
              ))}
            </div>
          );
        })()}
        {/* statuses to the left, hoverable */}
        {statuses.length > 0 && (
          <div className="cr-ov-statuses">
            {statuses.map((s) => (
              <span key={s} className="cr-ov-status" title={`${STATUSES[s].label}: ${STATUSES[s].blurb}`}>{STATUSES[s].icon}</span>
            ))}
          </div>
        )}
        {/* the d-pad, centred on the pawn */}
        {showMove && <CrawlerMovePad v={v} me={me} onSend={onSend} carry={carry} />}
        {/* small (no-text) action bars + courage next to the arrows; items under the courage bar */}
        <div className="cr-ov-side">
          <CrawlerActionBars v={v} me={me} onSend={onSend} compact names={false} />
          <CrawlerItemRow v={v} me={me} myTurn onSend={onSend} luckyArmed={lucky} onArmLucky={onArmLucky} />
        </div>
        {/* end turn, under the d-pad; End level appears under it during the post-boss lap */}
        <div className="cr-ov-end-col">
          <button className="cr-ov-end" onClick={() => onSend({ t: "endTurn" })} title="End your turn (e)">End turn{v.cleared && v.postBoss > 0 ? ` (${v.postBoss})` : ""}</button>
          {v.cleared && <button className="cr-ov-end cr-end-level" onClick={() => onSend({ t: "endLevel" })} title="End the level now">End level</button>}
        </div>
      </div>
    </div>
  );
}
