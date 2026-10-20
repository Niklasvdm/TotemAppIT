// The dice-pip die face for the crawler's dice pool.
import { DIE_PIPS } from "./data";

// DieFace is just the pip grid (a span, not a button), so it can be shown inside other buttons
// (e.g. the action-bar cost badges) without nesting <button>s. Keyed by value so it re-tumbles;
// `tumble` (a changing token) overrides the key so a reroll re-plays the tumble even when the
// same value comes up again (otherwise a same-value reroll looks like nothing happened).
export function DieFace({ value, tumble }: { value: number; tumble?: string | number }) {
  const on = new Set((DIE_PIPS[value] ?? []).map(([r, c]) => r * 3 + c));
  return (
    <span className="cr-die-face" key={tumble ?? value}>
      {Array.from({ length: 9 }, (_, i) => (
        <i key={i} className={on.has(i) ? "pip on" : "pip"} />
      ))}
    </span>
  );
}

// Die renders a value as pips. The face is keyed by value so it remounts and plays the
// tumble animation whenever the value changes (a roll or a reroll). `mini` is the small,
// read-only readout used by the board overlay: smaller, not a selectable control (no
// click, never shown as "disabled"), just a spent/unspent view of the pool.
export function Die({ value, spent, selected = false, usedFor, bonus = false, disabled = false, onClick, mini = false, rollKey }: {
  value: number; spent: boolean; selected?: boolean; usedFor: string; bonus?: boolean; disabled?: boolean; onClick?: () => void; mini?: boolean; rollKey?: string | number;
}) {
  const readout = mini && !onClick; // a non-interactive readout (no click handler)
  return (
    <button
      className={`cr-die${spent ? " spent" : ""}${selected ? " sel" : ""}${bonus ? " bonus" : ""}${mini ? " mini" : ""}${onClick ? " clickable" : ""}`}
      disabled={readout ? undefined : disabled}
      aria-hidden={readout ? true : undefined}
      tabIndex={readout ? -1 : undefined}
      onClick={onClick}
      title={bonus ? "acquired attack die" : spent ? usedFor : mini ? `die: ${value}` : "select this die"}
    >
      <DieFace value={value} tumble={rollKey} />
      {spent && <small>{usedFor[0]}</small>}
    </button>
  );
}
