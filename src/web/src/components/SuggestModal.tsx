import { useState } from "react";
import { suggestAnimal } from "../api";

// "Suggest an animal" — submits to the moderation queue (deduped + counted
// server-side). English only for now (TODO: it/nl).
export default function SuggestModal({ onClose }: { onClose: () => void }) {
  const [name, setName] = useState("");
  const [note, setNote] = useState("");
  const [state, setState] = useState<"idle" | "sending" | "done" | "error">("idle");
  const [error, setError] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setState("sending");
    try {
      await suggestAnimal(name.trim(), note.trim());
      setState("done");
    } catch (err) {
      setError(err instanceof Error ? err.message : "something went wrong");
      setState("error");
    }
  };

  return (
    <div className="modal-overlay" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <button className="modal-close" onClick={onClose} aria-label="close">
          ✕
        </button>

        {state === "done" ? (
          <div className="feedback-done">
            <h2 className="quiz-prompt">🙏 Thanks!</h2>
            <p>Your suggestion was sent for review. If others suggest it too, it rises up the list.</p>
            <button className="quiz-cta" onClick={onClose}>
              Close
            </button>
          </div>
        ) : (
          <form onSubmit={submit}>
            <h2 className="quiz-prompt">➕ Suggest an animal</h2>
            <p className="muted-note">
              Missing an animal? Suggest it and a moderator will review it. Duplicate suggestions are
              merged and counted.
            </p>
            <label className="field">
              <span>Animal name</span>
              <input
                autoFocus
                value={name}
                maxLength={60}
                placeholder="e.g. Capybara"
                onChange={(e) => setName(e.target.value)}
                required
              />
            </label>
            <label className="field">
              <span>Why? (optional)</span>
              <textarea
                value={note}
                maxLength={280}
                rows={3}
                placeholder="A short reason or a fun fact…"
                onChange={(e) => setNote(e.target.value)}
              />
            </label>
            {state === "error" && <p className="form-error">⚠ {error}</p>}
            <button className="quiz-cta" type="submit" disabled={state === "sending" || !name.trim()}>
              {state === "sending" ? "Sending…" : "Send suggestion"}
            </button>
          </form>
        )}
      </div>
    </div>
  );
}
