// The move d-pad: four arrows (N/E/S/W) that each auto-spend your lowest move die (or a free
// bonus step), greying out when that step is illegal (canStep mirrors the server). Shared by
// the mobile overlay (positioned on the pawn) and the desktop sidebar panel (inline). The
// .cr-dpad grid is static; the overlay adds absolute positioning in CSS.
import type { CrView, CrHero } from "./types";
import { DIR_ARROW, menuMins, canStep, lowestDie } from "./data";

const POS = ["up", "right", "down", "left"]; // N,E,S,W -> grid cell
const AXIS = ["north", "east", "south", "west"];

export function CrawlerMovePad({ v, me, onSend, carry = null }: { v: CrView; me: CrHero; onSend: (msg: object) => void; carry?: number | null }) {
  const mins = menuMins(me);
  const unspent = v.dice.some((d) => !d.spent);
  // While carrying, the arrows spend a die to move the carried ally one square (the server
  // validates the ally's step); otherwise they move you (or take a free bonus step).
  const carryDie = carry != null ? lowestDie(v.dice, mins[me.special] ?? 2) : -1;
  const act = (dir: number) => {
    if (carry != null) { if (carryDie >= 0) onSend({ t: "spend", die: carryDie, action: "carry", ally: carry, dir }); return; }
    if (v.bonusMoves > 0) { onSend({ t: "step", dir }); return; }
    const lo = lowestDie(v.dice, mins.move);
    if (lo >= 0) onSend({ t: "spend", die: lo, action: "move", dir });
  };
  const canDir = (dir: number) => (carry != null ? carryDie >= 0 : (v.bonusMoves > 0 || unspent) && canStep(v, me, dir));
  return (
    <div className={`cr-dpad${carry != null ? " carrying" : ""}`}>
      {[0, 1, 2, 3].map((dir) => (
        <button
          key={dir}
          className={`cr-arrow cr-arrow-${POS[dir]}`}
          disabled={!canDir(dir)}
          onClick={() => act(dir)}
          title={carry != null ? `Carry ${AXIS[dir]}` : `Move ${DIR_ARROW[dir]}`}
          aria-label={`${carry != null ? "Carry" : "Move"} ${AXIS[dir]}`}
        >
          {DIR_ARROW[dir]}
        </button>
      ))}
    </div>
  );
}
