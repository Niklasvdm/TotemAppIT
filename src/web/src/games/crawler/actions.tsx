// The per-die action menu shown when a die is selected: reroll, Move, Attack (melee or
// the Squid's ranged reach), the hero's special, and Courage.
import type { CrDie, CrHero, CrView } from "./types";
import { menuMins, specialInfo, dieIdx, COURAGE_CAP, DIRS, DIR_ARROW, attackTargets, effArmour } from "./data";

export function CrawlerActions({
  die,
  me,
  view,
  onAct,
  onReroll,
  onHover,
}: {
  die: CrDie;
  me: CrHero;
  view: CrView;
  onAct: (msg: object) => void;
  onReroll: () => void;
  onHover?: (key: string | null) => void;
}) {
  const mins = menuMins(me);
  const can = (action: string) => die.value >= (mins[action] ?? 99);
  const allies = view.heroes.filter((h) => h.alive && !h.downed && h.tile === me.tile);
  const others = allies.filter((a) => a.s !== me.s);
  const specLabel = specialInfo(me.special).name;
  const di = dieIdx(view, die);
  // Attack targets (shared with the keyboard 'f' shortcut): living monsters on your tile,
  // plus (for a ranged totem) ones a tile away. Only those the selected die can actually hit
  // are offered, since attacking with a die below the Armour is not allowed. effArmour is the
  // server's EFFECTIVE Armour for this hero, so the offer always matches what it will accept.
  const hittable = attackTargets(view, me).filter(({ m }) => die.value >= effArmour(m));

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
              title={`This ${die.value} meets Armour ${effArmour(m)}${effArmour(m) < m.armour ? ` (reduced from ${m.armour})` : effArmour(m) > m.armour ? ` (raised from ${m.armour})` : ""}: a hit.${ranged ? " Ranged, adjacent tile." : ""} Health ${m.health}/${m.maxHP}.`}
              onMouseEnter={() => onHover?.(`${tileId}-${m.id}`)}
              onMouseLeave={() => onHover?.(null)}
              onFocus={() => onHover?.(`${tileId}-${m.id}`)}
              onBlur={() => onHover?.(null)}
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
      {can(me.special) && me.special === "pilfer" && (
        <div className="cr-action-row">
          <span>{specLabel}:</span>
          {DIRS.map((_, d) => (
            <button key={d} className="game-btn" title="Pull a ground item from the adjacent tile that way into your pack." onClick={() => onAct({ t: "spend", die: di, action: "pilfer", dir: d })}>{DIR_ARROW[d]}</button>
          ))}
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
