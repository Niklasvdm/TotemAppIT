// The dice read-out, shared by the sidebar panel and the mobile overlay. Each unspent die
// shows a ↻ reroll control (on hover, and always on touch) that rerolls just that die for 1
// Courage, or free when a free reroll is available (Gambler's Glove / Lucky Paws). While the
// Lucky Charm is armed, clicking a die gives it the +1 instead.
import type { CrView, CrHero } from "./types";
import { Die } from "./Die";

export function CrawlerDiceRow({ v, me, className, lucky, onPickLucky, onReroll }: {
  v: CrView;
  me: CrHero;
  className: string;
  lucky: boolean;
  onPickLucky: (dieIndex: number) => void;
  onReroll: (dieIndex: number) => void;
}) {
  const canReroll = !!me.freeReroll || me.courage >= 1;
  return (
    <div className={`${className}${lucky ? " armed" : ""}`}>
      {v.dice.map((d, i) => ({ d, i })).sort((a, b) => a.d.value - b.d.value).map(({ d, i }) => (
        <span key={i} className="cr-die-wrap">
          <Die value={d.value} spent={d.spent} usedFor={d.usedFor} bonus={!!d.bonus} mini onClick={lucky && !d.spent ? () => onPickLucky(i) : undefined} />
          {!d.spent && !lucky && canReroll && (
            <button
              className="cr-die-reroll"
              title={me.freeReroll ? `Reroll this die (free: ${me.freeSrc || "🎲"})` : "Reroll this die (1 ✊)"}
              onClick={() => onReroll(i)}
            >
              ↻
            </button>
          )}
        </span>
      ))}
    </div>
  );
}
