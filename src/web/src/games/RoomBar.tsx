import { useState } from "react";
import { useTranslation } from "react-i18next";
import { InfoButton } from "./GameRules";

// The room-code header shared by the turn-based games, with the game title in the
// middle, a "how to play" button, and a copy-to-clipboard button.
export function RoomBar({
  code,
  title,
  infoSlug,
  onLeave,
}: {
  code: string;
  title?: string;
  infoSlug?: string;
  onLeave: () => void;
}) {
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
      {infoSlug && <InfoButton slug={infoSlug} />}
      <button className="game-btn ghost" onClick={onLeave}>
        {t("gameLeave")}
      </button>
    </div>
  );
}
