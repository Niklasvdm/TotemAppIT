// The action bars: every thing the hero can do is ONE uniform row, shaped like the Courage
// bar — [icon] [track] [die/cost] — and the rows are ordered by the die value they need
// (Move 1, Special 2, Attack n, Courage 5, Patch last). Courage is one of these bars: clicking
// it spends a die to raise Courage, its fill shows the level, and tick lines mark where the two
// specials unlock. Clicking a bar auto-spends the lowest usable die, auto-targeting. The mobile
// overlay uses the compact (no-text) variant. Shared by the sidebar panel and the overlay.
import type { ReactNode } from "react";
import type { CrView, CrHero } from "./types";
import { menuMins, COURAGE_CAP, COURAGE_UNLOCKS, OBJECT_CAP, attackTargets, effArmour, lowestDie, specialInfo, objInfo, useItemMsg, unlockInfo } from "./data";
import { DieFace } from "./Die";
import { ObjIcon } from "./sprites";
import { InfoDot } from "./widgets";

const UNLOCK_EMOJI = ["⭐", "🌟"]; // two distinct markers for the two Courage-tier specials
const SPECIAL_ICON: Record<string, string> = { brace: "🛡️", guard: "🛡️", mend: "❤️", carry: "🤝", pilfer: "🤏" };

type Tick = { pct: number; on: boolean; title: string };

// One uniform bar row. `fill` (0-100) draws the Courage-style fill; `ticks` mark positions on
// the track; `costDie` shows a pip die on the right (or `costText`). `compact` hides the label.
function StatBar({ icon, label, fill, ticks, costDie, costText, onClick, title, dim, compact }: {
  icon: string;
  label?: string;
  fill?: number;
  ticks?: Tick[];
  costDie?: number;
  costText?: string;
  onClick?: () => void;
  title: string;
  dim?: boolean;
  compact?: boolean;
}) {
  const inner = (
    <>
      <span className="cr-bar2-ico">{icon}</span>
      <span className="cr-bar2-track">
        {fill !== undefined && <span className="cr-bar2-fill" style={{ width: `${fill}%` }} />}
        {ticks?.map((t, i) => <i key={i} className={`cr-bar2-tick${t.on ? " on" : ""}`} style={{ left: `${t.pct}%` }} title={t.title} />)}
        {label && !compact && <span className="cr-bar2-label">{label}</span>}
      </span>
      <span className="cr-bar2-cost">
        {costDie !== undefined ? <span className="cr-cost-die"><DieFace value={costDie} /></span> : costText}
      </span>
    </>
  );
  return onClick
    ? <button className="cr-bar2" onClick={onClick} title={title}>{inner}</button>
    : <div className={`cr-bar2 info${dim ? " dim" : ""}`} title={title}>{inner}</div>;
}

export function CrawlerActionBars({ v, me, onSend, compact = false, names = true }: {
  v: CrView;
  me: CrHero;
  onSend: (msg: object) => void;
  compact?: boolean; // mobile: no text, smaller
  names?: boolean;   // show the unlock-marker names (desktop) vs emoji-only (mobile)
}) {
  const mins = menuMins(me);
  const unlocks = unlockInfo(me);
  const bars: { sort: number; node: ReactNode }[] = [];

  // Move (informational: you step with the arrows)
  const moveIdx = lowestDie(v.dice, mins.move);
  bars.push({ sort: mins.move, node: (
    <StatBar key="move" icon="👣" label="Move" costDie={mins.move} dim={moveIdx < 0} compact={compact} title="Move one square (use the arrows). Costs any die ≥1." />
  ) });

  // Signature special
  const sp = me.special;
  if (sp) {
    const info = specialInfo(sp);
    const auto = sp === "brace" || sp === "guard" || sp === "mend";
    const sidx = auto ? lowestDie(v.dice, mins[sp] ?? 99) : -1;
    const onClick = auto && sidx >= 0 ? () => {
      if (sp === "mend") {
        const onTile = v.heroes.filter((h) => h.alive && !h.downed && h.tile === me.tile);
        const tgt = onTile.reduce((best, h) => (h.health / h.maxHP < best.health / best.maxHP ? h : best), me);
        onSend({ t: "spend", die: sidx, action: "mend", ally: tgt.s });
      } else {
        onSend({ t: "spend", die: sidx, action: sp, ally: me.s });
      }
    } : undefined;
    bars.push({ sort: mins[sp] ?? 2, node: (
      <StatBar key="special" icon={SPECIAL_ICON[sp] ?? "✨"} label={info.name} costDie={mins[sp] ?? 2} onClick={onClick} dim={!onClick} compact={compact} title={`${info.name}: ${info.blurb}`} />
    ) });
  }

  // Attack (only when something is hittable)
  const targets = attackTargets(v, me);
  const atkIdx = lowestDie(v.dice, 1, (val) => targets.some((t) => val >= effArmour(t.m)));
  if (atkIdx >= 0) {
    const val = v.dice[atkIdx].value;
    const doAttack = () => {
      const hit = targets.filter((t) => val >= effArmour(t.m));
      if (!hit.length) return;
      const tgt = hit.reduce((b, t) => (t.m.health < b.m.health ? t : b));
      onSend({ t: "spend", die: atkIdx, action: "attack", monster: tgt.m.id, tile: tgt.tileId });
    };
    bars.push({ sort: val, node: (
      <StatBar key="attack" icon="⚔️" label="Attack" costDie={val} onClick={doAttack} compact={compact} title="Attack: spend your lowest die that beats a monster's Armour (hits the lowest-Health reachable one)." />
    ) });
  }

  // Courage: the clickable bar (fill = level) with the two unlock ticks + markers below
  const pct = Math.max(0, Math.min(100, Math.round((me.courage / COURAGE_CAP) * 100)));
  const couIdx = me.courage < COURAGE_CAP && !me.drained ? lowestDie(v.dice, mins.courage) : -1;
  const couClick = couIdx >= 0 ? () => onSend({ t: "spend", die: couIdx, action: "courage" }) : undefined;
  const ticks: Tick[] = COURAGE_UNLOCKS.map((u, i) => ({ pct: (u / COURAGE_CAP) * 100, on: me.courage >= u, title: `${unlocks[i].name} unlocks at ${u} Courage` }));
  bars.push({ sort: mins.courage, node: (
    <div key="courage" className="cr-cou2">
      <StatBar icon="✊" fill={pct} ticks={ticks} label={`${me.courage}/${COURAGE_CAP}`} costDie={mins.courage} onClick={couClick} title={me.drained ? "Drained: cannot gain Courage" : me.courage >= COURAGE_CAP ? "Courage is maxed (10)" : "Courage: spend a die ≥5 to gain +1 (cap 10)."} />
      <div className="cr-cou-marks">
        {([0, 1] as const).map((i) => (
          <span key={i} className={`cr-cou-mark${me.unlocked[i] ? " on" : ""}`}>
            <span className="cr-cou-mark-ico">{UNLOCK_EMOJI[i]}</span>
            {names && <span className="cr-cou-mark-name">{unlocks[i].name}</span>}
            <InfoDot text={`${unlocks[i].name} (at ${COURAGE_UNLOCKS[i]} Courage): ${unlocks[i].blurb}`} />
          </span>
        ))}
      </div>
    </div>
  ) });

  // Patch up (no die: 2 Courage), ordered last
  if (me.courage >= 2 && me.health < me.maxHP) {
    bars.push({ sort: 99, node: (
      <StatBar key="patch" icon="🩹" label="Patch up" costText="✊2" onClick={() => onSend({ t: "heal" })} compact={compact} title="Patch up: trade 2 Courage for 1 Health (no die)." />
    ) });
  }

  bars.sort((a, b) => a.sort - b.sort);
  return <div className="cr-bars2">{bars.map((b) => b.node)}</div>;
}

// The hero's object slots (every slot, so free spaces are visible). A filled slot is a quick-
// use button plus a ⤓ drop button; an empty slot shows the gap. The Lucky Charm arms a
// "pick a die" mode (onArmLucky) instead of firing, since it needs a target die.
export function CrawlerItemRow({ v, me, myTurn, sel = null, onSend, luckyArmed = false, onArmLucky }: {
  v: CrView;
  me: CrHero;
  myTurn: boolean;
  sel?: number | null;
  onSend: (msg: object) => void;
  luckyArmed?: boolean;
  onArmLucky?: () => void;
}) {
  const cap = me.slots || OBJECT_CAP;
  const canAct = myTurn && !me.downed;
  const snatch = me.special === "pilfer"; // Snatch totems: their items get an extra pink glow
  return (
    <div className="cr-items" title={`${me.objects.length}/${cap} object slots`}>
      {Array.from({ length: cap }, (_, i) => {
        const slug = me.objects[i];
        if (!slug) return <span key={i} className="cr-item empty" />;
        const info = objInfo(slug);
        const isLucky = slug === "lucky-charm";
        const luckyUsable = isLucky && canAct && !!me.luckyReady && !!onArmLucky;
        const msg = canAct && !isLucky ? useItemMsg(v, me, slug, sel) : null;
        const usable = isLucky ? luckyUsable : !!msg;
        const onUse = isLucky ? () => onArmLucky?.() : () => { if (msg) onSend(msg); };
        return (
          <span key={i} className={`cr-item filled${usable ? " usable" : ""}${snatch ? " snatch" : ""}${isLucky && luckyArmed ? " armed" : ""}`}>
            <button
              className="cr-item-use"
              disabled={!usable}
              title={isLucky ? `${info.name}: ${info.effect}${luckyUsable ? " — click, then click a die to +1 it" : " (not usable now)"}` : `${info.name}: ${info.effect}${msg ? "" : " (not usable now)"}`}
              onClick={onUse}
            >
              <ObjIcon slug={slug} size={22} />
            </button>
            {canAct && (
              <button className="cr-item-drop" title={`Drop ${info.name} on your square`} onClick={() => onSend({ t: "drop", obj: slug })}>⤓</button>
            )}
          </span>
        );
      })}
    </div>
  );
}
