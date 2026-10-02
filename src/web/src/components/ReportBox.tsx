import { useState } from "react";
import { reportAnimal, type ReportReason } from "../api";

const REASONS: { value: ReportReason; label: string }[] = [
  { value: "incorrect", label: "Something is wrong / inaccurate" },
  { value: "unknown", label: "Too obscure / nobody knows it" },
  { value: "poor_description", label: "Poor or missing description" },
  { value: "other", label: "Other" },
];

// A prominent red flag in the top-right corner of the animal page. Many of these
// are very Flemish-specific animals, so readers can flag the odd ones.
export default function ReportBox({ slug }: { slug: string }) {
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState<ReportReason>("unknown");
  const [note, setNote] = useState("");
  const [state, setState] = useState<"idle" | "sending" | "done" | "error">("idle");
  const [error, setError] = useState("");

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setState("sending");
    try {
      await reportAnimal(slug, reason, note.trim());
      setState("done");
    } catch (err) {
      setError(err instanceof Error ? err.message : "something went wrong");
      setState("error");
    }
  };

  return (
    <>
      <button
        className="report-flag"
        onClick={() => setOpen(true)}
        title="Report this animal"
        aria-label="Report this animal"
      >
        ⚑
      </button>

      {open && (
        <div className="modal-overlay" onClick={() => setOpen(false)}>
          <div className="modal report-modal" onClick={(e) => e.stopPropagation()}>
            <button className="modal-close" onClick={() => setOpen(false)} aria-label="close">
              ✕
            </button>

            {state === "done" ? (
              <div className="feedback-done">
                <h2 className="report-title">⚑ Thanks</h2>
                <p>Sent for review.</p>
                <button className="danger-btn" onClick={() => setOpen(false)}>
                  Close
                </button>
              </div>
            ) : (
              <form onSubmit={submit}>
                <h2 className="report-title">⚑ Report this animal</h2>
                <p className="muted-note">
                  Something off about this totem? Lots of these are very Flemish-specific.
                </p>
                <label className="field">
                  <span>What's wrong?</span>
                  <select value={reason} onChange={(e) => setReason(e.target.value as ReportReason)}>
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
                    rows={2}
                    placeholder="Anything that helps…"
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
