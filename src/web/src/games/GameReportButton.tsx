import { useState } from "react";
import { reportGame, type GameReportReason } from "../api";

const REASONS: { value: GameReportReason; label: string }[] = [
  { value: "bug", label: "Something is broken / crashed" },
  { value: "rules", label: "A rule works wrong" },
  { value: "unclear", label: "Confusing / hard to understand" },
  { value: "other", label: "Other" },
];

// A small flag button (for the game bar) that opens a "report a problem" modal
// for the current game, mirroring the animals' ReportBox. `game` is the backend
// slug (the-mind, just-one, crawler, …).
export function GameReportButton({ game, className }: { game: string; className?: string }) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState<GameReportReason>("bug");
  const [note, setNote] = useState("");
  const [state, setState] = useState<"idle" | "sending" | "done" | "error">("idle");
  const [error, setError] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setState("sending");
    try {
      await reportGame(game, reason, note.trim());
      setState("done");
    } catch (err) {
      setError(err instanceof Error ? err.message : "something went wrong");
      setState("error");
    }
  };

  const close = () => {
    setOpen(false);
    // reset for next time, after the modal is gone
    setTimeout(() => { setState("idle"); setNote(""); setReason("bug"); setError(""); }, 200);
  };

  return (
    <>
      <button
        className={className ?? "report-flag"}
        onClick={() => setOpen(true)}
        title="Report a problem with this game"
        aria-label="Report a problem with this game"
      >
        ⚑
      </button>

      {open && (
        <div className="modal-overlay" onClick={close}>
          <div className="modal report-modal" onClick={(e) => e.stopPropagation()}>
            <button className="modal-close" onClick={close} aria-label="close">
              ✕
            </button>

            {state === "done" ? (
              <div className="feedback-done">
                <h2 className="report-title">⚑ Thanks</h2>
                <p>Sent for review.</p>
                <button className="danger-btn" onClick={close}>
                  Close
                </button>
              </div>
            ) : (
              <form onSubmit={submit}>
                <h2 className="report-title">⚑ Report a problem</h2>
                <p className="muted-note">
                  Something off with this game? Tell us so it can be fixed.
                </p>
                <label className="field">
                  <span>What's wrong?</span>
                  <select value={reason} onChange={(e) => setReason(e.target.value as GameReportReason)}>
                    {REASONS.map((r) => (
                      <option key={r.value} value={r.value}>
                        {r.label}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="field">
                  <span>Details (optional)</span>
                  <textarea
                    value={note}
                    maxLength={280}
                    rows={3}
                    placeholder="What happened, and what did you expect?"
                    onChange={(e) => setNote(e.target.value)}
                  />
                </label>
                {state === "error" && <p className="form-error">⚠ {error}</p>}
                <button className="danger-btn" type="submit" disabled={state === "sending"}>
                  {state === "sending" ? "Sending…" : "Send report"}
                </button>
              </form>
            )}
          </div>
        </div>
      )}
    </>
  );
}
