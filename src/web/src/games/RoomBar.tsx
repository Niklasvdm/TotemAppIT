import { useState } from "react";
import { useTranslation } from "react-i18next";

// The room-code header shared by the turn-based games, with the game title in the
// middle and a copy-to-clipboard button (codes get read aloud / shared).
export function RoomBar({ code, title, onLeave }: { code: string; title?: string; onLeave: () => void }) {
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
      {title && <strong className="game-bar-title">{title}</strong>}
      <button className="game-btn ghost" onClick={onLeave}>
        {t("gameLeave")}
      </button>
    </div>
  );
}
