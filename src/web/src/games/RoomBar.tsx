import { useState } from "react";
import { useTranslation } from "react-i18next";
import { InfoButton } from "./GameRules";
import { GameReportButton } from "./GameReportButton";

// The room-code header shared by the turn-based games, with the game title in the
// middle, a "how to play" button, a "report a problem" flag, and a copy button.
export function RoomBar({
  code,
  title,
  infoSlug,
  gameSlug,
  onLeave,
}: {
  code: string;
  title?: string;
  infoSlug?: string;
  gameSlug?: string; // backend game slug, enables the report flag
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
      {gameSlug && <GameReportButton game={gameSlug} className="game-report-flag" />}
      <button className="game-btn ghost" onClick={onLeave}>
        {t("gameLeave")}
      </button>
    </div>
  );
}
