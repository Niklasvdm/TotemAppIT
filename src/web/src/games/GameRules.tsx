import { useState } from "react";
import { GAME_RULES, type GameRules } from "./rules";

// A small ⓘ button that opens a "how to play" modal for one game. Used on the
// games index (over each card) and in-game (in the room bar).
export function InfoButton({ slug, className = "" }: { slug: string; className?: string }) {
  const [open, setOpen] = useState(false);
  const rules = GAME_RULES[slug];
  if (!rules) return null;
  return (
    <>
      <button
        type="button"
        className={`info-btn ${className}`}
        title={`How to play ${rules.title}`}
        aria-label={`How to play ${rules.title}`}
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          setOpen(true);
        }}
      >
        i
      </button>
      {open && <RulesModal rules={rules} onClose={() => setOpen(false)} />}
    </>
  );
}

function RulesModal({ rules, onClose }: { rules: GameRules; onClose: () => void }) {
  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal game-rules" onClick={(e) => e.stopPropagation()}>
        <button className="modal-close" onClick={onClose} aria-label="Close">
          ✕
        </button>
        <h2>
          {rules.icon} {rules.title}
        </h2>
        <p className="muted-note">
          {rules.players} · {rules.summary}
        </p>
        <h3>How to play</h3>
        <ol className="game-rules-steps">
          {rules.steps.map((s, i) => (
            <li key={i}>{s}</li>
          ))}
        </ol>
        <h3>Example</h3>
        <p className="game-rules-example">{rules.example}</p>
      </div>
    </div>
  );
}
