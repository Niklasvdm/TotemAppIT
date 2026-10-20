// The expanded "my" block in the sidebar, on your turn. Top: statuses (left, hoverable), the
// sprite, then hearts (vertical) + shields on the right. Below: the dice read-out, the move
// d-pad, the action blocks, the Courage bar with its unlock markers, your items, and a footer
// (portal / revive / End turn). During the lay phase it shows the drawn-tile card instead.
import type { ReactNode } from "react";
import type { CrView, CrHero, CrTile } from "./types";
import { heroStatuses, STATUSES } from "./data";
import { TotemSprite } from "./sprites";
import { CrawlerDiceRow } from "./dicerow";
import { CrawlerMovePad } from "./movepad";
import { CrawlerActionBars, CrawlerItemRow } from "./actionbars";

export function CrawlerHeroPanel({ v, me, lost, onSend, layNode, onPortalCenter, portalDests, portalNo, lucky, onArmLucky, onPickLucky, onReroll, carry, onHoverTarget }: {
  v: CrView;
  me: CrHero;
  lost: boolean;
  onSend: (msg: object) => void;
  layNode: ReactNode; // the lay-phase card (shared action panel), shown while laying
  onPortalCenter: boolean;
  portalDests: CrTile[];
  portalNo: (id: number) => number;
  lucky: boolean; // Lucky Charm armed: dice are clickable to receive the +1
  onArmLucky: () => void;
  onPickLucky: (dieIndex: number) => void;
  onReroll: (dieIndex: number) => void;
  carry: number | null; // Carry armed: the arrows move this ally (its sprite shows by the dice)
  onHoverTarget?: (key: string | null) => void; // hovering the Attack bar highlights its target on the board
}) {
  const statuses = heroStatuses(me);
  const carried = carry != null ? v.heroes.find((h) => h.s === carry) : undefined;
  const hasDie = v.dice.some((d) => !d.spent);
  const downedAllies = v.heroes.filter((h) => h.downed && h.tile === me.tile && h.s !== me.s);

  return (
    <div className="cr-hp panel">
      <div className="cr-hp-name">
        <strong>{me.n} (you)</strong>
        <span className="muted-note">{me.totem}{me.downed ? " · down" : ""}</span>
      </div>
      <div className="cr-hp-head">
        {/* diseases / weaknesses on the left, hoverable */}
        <div className="cr-hp-statuses">
          {statuses.map((s) => (
            <span key={s} className="cr-hp-status" title={`${STATUSES[s].label}: ${STATUSES[s].blurb}`}>{STATUSES[s].icon}</span>
          ))}
        </div>
        <TotemSprite slug={me.totem} size={56} dead={me.downed || lost} />
        {/* hearts stacked on the right; shields alongside */}
        <div className="cr-hp-hearts" title={`${me.health}/${me.maxHP} health`}>
          {Array.from({ length: me.maxHP }, (_, i) => (
            <span key={i} className="cr-pip-heart">{i < me.health ? "❤️" : "🤍"}</span>
          ))}
        </div>
        {me.shields > 0 && (
          <div className="cr-hp-shields" title={`${me.shields} shields`}>
            {Array.from({ length: me.shields }, (_, i) => <span key={i} className="cr-pip-shield">🛡️</span>)}
          </div>
        )}
      </div>

      {v.step === "lay" ? (
        <div className="cr-hp-lay">{layNode}</div>
      ) : (
        <>
          {/* carrying: the ally's sprite replaces the dice; pick a direction with the arrows */}
          {carried ? (
            <div className="cr-carry-note">
              <TotemSprite slug={carried.totem} size={34} />
              <span className="muted-note">Carrying {carried.n} — pick a direction (tap them again to stop)</span>
            </div>
          ) : (
            <CrawlerDiceRow v={v} me={me} className="cr-hp-dice" lucky={lucky} onPickLucky={onPickLucky} onReroll={onReroll} />
          )}

          <CrawlerMovePad v={v} me={me} onSend={onSend} carry={carry} />
          <CrawlerActionBars v={v} me={me} onSend={onSend} onHoverTarget={onHoverTarget} />

          <CrawlerItemRow v={v} me={me} myTurn onSend={onSend} luckyArmed={lucky} onArmLucky={onArmLucky} />

          {/* footer: portal, revive, End turn / End level */}
          <div className="cr-hp-footer">
            {onPortalCenter && portalDests.length > 0 && (
              <div className="cr-action-row">
                <span>🌀 Portal to:</span>
                {portalDests.map((ti) => (
                  <button key={ti.id} className="game-btn" onClick={() => onSend({ t: "portal", tile: ti.id })}>→ {portalNo(ti.id)}</button>
                ))}
              </div>
            )}
            {downedAllies.map((a) => (
              <div className="cr-action-row" key={a.s}>
                <span>⛑ {a.n} down:</span>
                <button className="game-btn" disabled={!hasDie} title={hasDie ? "Spend a die to revive (v)" : "needs an unspent die"} onClick={() => onSend({ t: "revive", ally: a.s })}>revive</button>
              </div>
            ))}
            {v.cleared ? (
              <>
                <span className="cr-cleared-note">🏆 Boss defeated! Grab loot, then descend.</span>
                <button className="game-btn" onClick={() => onSend({ t: "endTurn" })}>End turn{v.postBoss > 0 ? ` (${v.postBoss})` : ""}</button>
                <button className="game-btn cr-end-level" onClick={() => onSend({ t: "endLevel" })}>End level</button>
              </>
            ) : (
              <button className="game-btn" onClick={() => onSend({ t: "endTurn" })}>End turn</button>
            )}
          </div>
        </>
      )}
    </div>
  );
}
