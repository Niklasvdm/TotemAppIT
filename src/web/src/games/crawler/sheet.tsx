// The player's own character sheet: totem sprite, health/courage bars, the action menu
// (die thresholds), the two Courage specials, and the object slots (use / drop).
import type { CrHero, CrView } from "./types";
import {
  menuMins, specialInfo, unlockInfo, COURAGE_UNLOCKS, COURAGE_CAP, OBJECT_CAP, objInfo,
} from "./data";
import { TotemSprite, ObjIcon } from "./sprites";
import { InfoDot } from "./widgets";

export function CharacterSheet({ me, lost, view, myTurn, selDie, onSend }: {
  me: CrHero;
  lost: boolean;
  view: CrView;
  myTurn: boolean;
  selDie: number | null;
  onSend: (msg: object) => void;
}) {
  const mins = menuMins(me);
  const spec = specialInfo(me.special);
  const unlocks = unlockInfo(me);
  const myTile = view.tiles.find((t) => t.id === me.tile);
  const grenadeTargets = (myTile?.monsters ?? []).filter((m) => m.alive && m.health <= 1 && !m.isBoss);
  const hasPortal = view.tiles.some((t) => t.kind === "portal");
  // Whether a held object can be used right now (from its slot), and what happens.
  const usable = (slug: string): boolean => {
    if (!myTurn || !me.alive) return false;
    switch (slug) {
      case "lucky-charm": return selDie != null;
      case "grenade": return grenadeTargets.length > 0;
      case "medkit": return me.health < me.maxHP || !!me.electrified;
      case "battery": return me.courage < COURAGE_CAP;
      case "recall-whistle": return hasPortal;
      case "torch": return (myTile?.hazards?.length ?? 0) > 0; // clears this tile's hazards
      case "courage-horn": case "beacon": case "sap-seed": return true;
      default: return false; // other permanents are passive
    }
  };
  const useSlot = (slug: string) => {
    if (slug === "lucky-charm" && selDie != null) onSend({ t: "lucky", die: selDie });
    else if (slug === "grenade" && grenadeTargets.length > 0) onSend({ t: "use", obj: "grenade", monster: grenadeTargets[0].id });
    else onSend({ t: "use", obj: slug });
  };
  const hpPct = Math.max(0, Math.min(100, Math.round((me.health / me.maxHP) * 100)));
  const couPct = Math.max(0, Math.min(100, Math.round((me.courage / COURAGE_CAP) * 100)));
  // A special auto-activates at its Courage threshold and switches off below it.
  const specClass = (i: 0 | 1) => (me.unlocked[i] ? "ready" : "");
  return (
    <div className="panel cr-sheet">
      <div className="cr-sheet-head">
        <TotemSprite slug={me.totem} size={56} dead={!me.alive || lost} />
        <div className="cr-sheet-id">
          <strong>{me.n}</strong>
          <span className="muted-note">{me.totem}{me.alive ? "" : " · down"}</span>
        </div>
        <div className="cr-sheet-stats">
          <div className="cr-bar-row">
            <span className="cr-bar-label">❤ {me.health}/{me.maxHP}</span>
            <div className="cr-bar cr-hpbar"><span style={{ width: `${hpPct}%` }} /></div>
          </div>
          <div className="cr-bar-row">
            <span className="cr-bar-label">✊ {me.courage}/{COURAGE_CAP}</span>
            <div className="cr-bar cr-courbar">
              <span style={{ width: `${couPct}%` }} />
              <i className="cr-bar-tick" style={{ left: `${(COURAGE_UNLOCKS[0] / COURAGE_CAP) * 100}%` }} title={`${unlocks[0].name} at ${COURAGE_UNLOCKS[0]}`} />
              <i className="cr-bar-tick" style={{ left: `${(COURAGE_UNLOCKS[1] / COURAGE_CAP) * 100}%` }} title={`${unlocks[1].name} at ${COURAGE_UNLOCKS[1]}`} />
            </div>
          </div>
          <span className="muted-note">🛡 {me.shields} shields</span>
        </div>
      </div>
      <div className="cr-menu">
        <span className="cr-menu-item">Move <b>≥{mins.move}</b></span>
        {me.special && (
          <span className="cr-menu-item">{spec.name} <b>≥{mins[me.special]}</b> <InfoDot text={spec.blurb} /></span>
        )}
        <span className="cr-menu-item">Attack <InfoDot text="Spend any die that meets the monster's Armour as an attack. A die below the Armour cannot land, so it is not offered; spend low dice on Move or your Special." /></span>
        <span className="cr-menu-item">Courage <b>≥{mins.courage}</b></span>
      </div>
      <div className="cr-specials">
        <span className={specClass(0)} key={`u0-${me.unlocked[0]}`}>
          ★ {unlocks[0].name} {me.unlocked[0] ? "⚡active" : `(${COURAGE_UNLOCKS[0]}✊)`} <InfoDot text={unlocks[0].blurb} />
        </span>
        <span className={specClass(1)} key={`u1-${me.unlocked[1]}`}>
          ★ {unlocks[1].name} {me.unlocked[1] ? "⚡active" : `(${COURAGE_UNLOCKS[1]}✊)`} <InfoDot text={unlocks[1].blurb} />
        </span>
      </div>
      <div className="cr-slots">
        <span className="cr-slots-label">🎒 Objects {me.objects.length}/{me.slots || OBJECT_CAP}</span>
        {Array.from({ length: me.slots || OBJECT_CAP }, (_, i) => {
          const slug = me.objects[i];
          if (!slug) {
            return (
              <span key={i} className="cr-slot empty">
                <InfoDot text="An empty object slot. Fill it with loot dropped by robots, from a Vault, or bought at the kangaroo shop." />
              </span>
            );
          }
          const info = objInfo(slug);
          const canUse = usable(slug);
          const tip = `${info.name}: ${info.effect}${info.perm ? " (permanent)" : " (one-use)"}`;
          return (
            <span key={i} className={`cr-slot filled${canUse ? " usable" : ""}`}>
              {canUse ? (
                <button className="cr-slot-btn" title={`Use: ${info.name}`} onClick={() => useSlot(slug)}><ObjIcon slug={slug} size={26} /></button>
              ) : (
                <ObjIcon slug={slug} size={26} />
              )}
              {myTurn && me.alive && (
                <button className="cr-slot-drop" title={`Drop ${info.name} on your square`} onClick={() => onSend({ t: "drop", obj: slug })}>⤓</button>
              )}
              <InfoDot text={tip} />
            </span>
          );
        })}
      </div>
    </div>
  );
}
