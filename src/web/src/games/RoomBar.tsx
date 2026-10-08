import { useState } from "react";
import { useTranslation } from "react-i18next";

// The room-code header shared by the turn-based games, with a copy-to-clipboard
// button (codes get read aloud / shared, so copying is handy).
export function RoomBar({ code, onLeave }: { code: string; onLeave: () => void }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 1200);
    } catch {
      /* clipboard blocked (insecure context / denied) — ignore */
    }
  };

  return (
    <div className="game-bar panel">
      <button className="game-code game-code-copy" onClick={copy} title="Copy room code">
        <small>
          {t("gameRoomCode")} {copied ? "✓" : "📋"}
        </small>
        <strong>{code}</strong>
      </button>
      <button className="game-btn ghost" onClick={onLeave}>
        {t("gameLeave")}
      </button>
    </div>
  );
}
