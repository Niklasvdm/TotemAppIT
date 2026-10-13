// The dice-pip die face for the crawler's dice pool.
import { DIE_PIPS } from "./data";

// Die renders a value as pips. The face is keyed by value so it remounts and plays the
// tumble animation whenever the value changes (a roll or a reroll).
export function Die({ value, spent, selected, usedFor, bonus = false, disabled, onClick }: {
  value: number; spent: boolean; selected: boolean; usedFor: string; bonus?: boolean; disabled: boolean; onClick: () => void;
}) {
  const on = new Set((DIE_PIPS[value] ?? []).map(([r, c]) => r * 3 + c));
  return (
    <button
      className={`cr-die${spent ? " spent" : ""}${selected ? " sel" : ""}${bonus ? " bonus" : ""}`}
      disabled={disabled}
      onClick={onClick}
      title={bonus ? "acquired attack die" : spent ? usedFor : "select this die"}
    >
      <span className="cr-die-face" key={value}>
        {Array.from({ length: 9 }, (_, i) => (
          <i key={i} className={on.has(i) ? "pip on" : "pip"} />
        ))}
      </span>
      {spent && <small>{usedFor[0]}</small>}
    </button>
  );
}
