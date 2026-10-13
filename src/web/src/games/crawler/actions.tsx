// The per-die action menu shown when a die is selected: reroll, Move, Attack (melee or
// the Squid's ranged reach), the hero's special, and Courage.
import { effectiveArmour } from "./logic";
import type { CrDie, CrHero, CrMonster, CrView } from "./types";
import { menuMins, specialInfo, dieIdx, CR_DELTA, COURAGE_CAP, DIRS, DIR_ARROW } from "./data";

export function CrawlerActions({
  die,
  me,
  view,
  onAct,
  onReroll,
}: {
  die: CrDie;
  me: CrHero;
  view: CrView;
  onAct: (msg: object) => void;
  onReroll: () => void;
}) {
  const mins = menuMins(me);
  const can = (action: string) => die.value >= (mins[action] ?? 99);
  const myTile = view.tiles.find((x) => x.id === me.tile);
  const allies = view.heroes.filter((h) => h.alive && h.tile === me.tile);
  const others = allies.filter((a) => a.s !== me.s);
  const specLabel = specialInfo(me.special).name;
  const di = dieIdx(view, die);
  // Arrow Squid's Barbed Shot (first special, active by Courage) lowers a target's Armour
  // by 1; a die lands only if it meets the effective Armour (mirrors the server).
  const barbed = me.totem === "pijlinktvis" && me.unlocked[0];
  const effArmour = (m: CrMonster) => effectiveArmour(m.armour, barbed);
  // Attack targets: living monsters on your tile, plus (for a ranged totem) ones on a
  // tile reachable through an open edge of yours. Only those the selected die can
  // actually hit are offered: attacking with a die below the Armour is not allowed.
  const targets: { tileId: number; m: CrMonster; ranged: boolean }[] = [];
  (myTile?.monsters ?? []).filter((m) => m.alive).forEach((m) => targets.push({ tileId: myTile!.id, m, ranged: false }));
  if (me.ranged && myTile) {
    CR_DELTA.forEach(([dx, dy], d) => {
      if (!myTile.edges[d]) return;
      const nt = view.tiles.find((t) => t.x === myTile.x + dx && t.y === myTile.y + dy);
      // The boss can't be sniped from an adjacent tile (the server rejects it), so don't
      // offer it as a ranged target at all; only ordinary robots next door are hittable.
      nt?.monsters.filter((m) => m.alive && !m.isBoss).forEach((m) => targets.push({ tileId: nt.id, m, ranged: true }));
    });
  }
  const hittable = targets.filter(({ m }) => die.value >= effArmour(m));

  return (
    <div className="cr-actions">
      <div className="cr-action-row">
        {me.freeReroll ? (
          <button className="game-btn ghost" title="A free reroll is available this turn" onClick={onReroll}>↻ reroll ({me.freeSrc || "🎲"} free)</button>
        ) : (
          <button className="game-btn ghost" disabled={me.courage < 1} onClick={onReroll}>↻ reroll (1✊)</button>
        )}
      </div>
      {can("move") && (
        <div className="cr-action-row">
          <span>Move:</span>
          {DIRS.map((_, d) => (
            <button key={d} className="game-btn" onClick={() => onAct({ t: "spend", die: di, action: "move", dir: d })}>{DIR_ARROW[d]}</button>
          ))}
        </div>
      )}
      {hittable.length > 0 && (
        <div className="cr-action-row">
          <span>Attack:</span>
          {hittable.map(({ tileId, m, ranged }) => (
            <button
              key={`${tileId}-${m.id}`}
              className="game-btn"
              title={`This ${die.value} meets Armour ${effArmour(m)}${barbed && effArmour(m) < m.armour ? " (Barbed Shot: -1)" : ""}: a hit.${ranged ? " Ranged, adjacent tile." : ""} Health ${m.health}/${m.maxHP}.`}
              onClick={() => onAct({ t: "spend", die: di, action: "attack", monster: m.id, tile: tileId })}
            >
              {ranged ? "🎯" : m.isBoss ? "👑" : "👹"} A{effArmour(m)} · ❤{m.health}/{m.maxHP} ✓
            </button>
          ))}
        </div>
      )}
      {can(me.special) && (me.special === "brace" || me.special === "mend") && (
        <div className="cr-action-row">
          <span>{specLabel}:</span>
          {allies.map((a) => (
            <button key={a.s} className="game-btn" onClick={() => onAct({ t: "spend", die: di, action: me.special, ally: a.s })}>
              {a.s === me.s ? "self" : a.n}
            </button>
          ))}
        </div>
      )}
      {can(me.special) && me.special === "carry" && others.map((a) => (
        <div className="cr-action-row" key={a.s}>
          <span>Carry {a.n}:</span>
          {DIRS.map((_, d) => (
            <button key={d} className="game-btn" onClick={() => onAct({ t: "spend", die: di, action: "carry", ally: a.s, dir: d })}>{DIR_ARROW[d]}</button>
          ))}
        </div>
      ))}
      {can(me.special) && me.special === "guard" && (
        <div className="cr-action-row">
          <span>{specLabel}:</span>
          <button className="game-btn" onClick={() => onAct({ t: "spend", die: di, action: "guard", ally: me.s })}>self (+1🛡)</button>
        </div>
      )}
      {can("courage") && me.courage < COURAGE_CAP && (
        <div className="cr-action-row">
          <button className="game-btn" onClick={() => onAct({ t: "spend", die: di, action: "courage" })}>✊ Courage +1</button>
        </div>
      )}
      {can("courage") && me.courage >= COURAGE_CAP && <div className="cr-action-row muted-note">✊ Courage is maxed ({COURAGE_CAP}).</div>}
    </div>
  );
}
