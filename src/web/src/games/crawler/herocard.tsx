// One hero's party card: name, the ❤/✊/🛡 line, active statuses, totem + unlock stars, and the
// object pack. Shared by the classic party list and the compact left rail (where it is the
// compressed read-out of each teammate, and of you when it is not your turn).
import type { CrHero } from "./types";
import { heroStatuses, STATUSES, objInfo, OBJECT_CAP } from "./data";
import { ObjIcon } from "./sprites";

export function HeroCard({ h, you, turn }: { h: CrHero; you: number; turn: number }) {
  return (
    <div className={`cr-hero${h.s === turn ? " active" : ""}${h.downed || !h.alive ? " down" : ""}`}>
      <strong>{h.n}{h.s === you ? " (you)" : ""}{h.downed ? " ⛑ DOWN" : ""}</strong>
      <span>❤ {h.health}/{h.maxHP} · ✊ {h.courage} · 🛡 {h.shields}</span>
      {heroStatuses(h).length > 0 && (
        <span className="cr-status-row">
          {heroStatuses(h).map((s) => (
            <span key={s} className="cr-status" title={`${STATUSES[s].label}: ${STATUSES[s].blurb}`}>{STATUSES[s].icon}</span>
          ))}
        </span>
      )}
      <span className="muted-note">{h.totem}{h.unlocked[0] ? " ·★" : ""}{h.unlocked[1] ? "★" : ""}</span>
      <span className="cr-pack-row" title={`${h.objects.length}/${h.slots || OBJECT_CAP} object slots`}>
        {Array.from({ length: h.slots || OBJECT_CAP }, (_, i) => {
          const o = h.objects[i];
          return o
            ? <span key={i} className="cr-pack-cell" title={`${objInfo(o).name}: ${objInfo(o).effect}`}><ObjIcon slug={o} size={18} /></span>
            : <span key={i} className="cr-pack-cell empty" />;
        })}
      </span>
    </div>
  );
}
