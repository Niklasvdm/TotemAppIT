// Sprite atoms for the crawler: the totem avatar (from the shared Bomberman atlas), the
// object icon, the enemy sprite, and the walk-animation hook that drives the board pawns.
import { useEffect, useRef, useState } from "react";
import { type CrPose, POSE_BASE, POSE_LEN, poseFromDelta } from "./logic";
import type { CrMonster, CrView, SpriteFor } from "./types";
import {
  SPRITE_ROWS, ATLAS_COLS, ATLAS_ROWS, WALK_FRAME_MS, WALK_STEP_MS, ENEMY_ART, objInfo,
} from "./data";

// TotemSprite draws one totem frame from the shared atlas via CSS background
// positioning: a walk pose + frame (cycled while moving), mirrored for a leftward side
// pose, or the defeated frame when dead. Falls back to an emoji for unsprited totems.
export function TotemSprite({ slug, size = 40, pose = "front", frame = 0, flip = false, dead = false }: {
  slug: string; size?: number; pose?: CrPose; frame?: number; flip?: boolean; dead?: boolean;
}) {
  const row = SPRITE_ROWS.indexOf(slug);
  if (row < 0) return <span style={{ fontSize: size * 0.8 }}>{dead ? "💀" : "🧭"}</span>;
  const col = dead ? ATLAS_COLS - 1 : POSE_BASE[pose] + (frame % POSE_LEN[pose]);
  return (
    <span
      className="cr-sprite"
      style={{
        width: size,
        height: size,
        backgroundImage: "url(/sprites/totems.png)",
        backgroundSize: `${ATLAS_COLS * size}px ${ATLAS_ROWS * size}px`,
        backgroundPosition: `${-col * size}px ${-row * size}px`,
        transform: !dead && flip ? "scaleX(-1)" : undefined,
      }}
    />
  );
}

// ObjIcon shows the object's art, falling back to its emoji if the image is missing.
export function ObjIcon({ slug, size = 22 }: { slug: string; size?: number }) {
  const [broken, setBroken] = useState(false);
  const info = objInfo(slug);
  if (broken) return <span style={{ fontSize: size * 0.9 }}>{info.emoji}</span>;
  return (
    <img
      className="cr-obj-icon"
      src={`/sprites/crawler/objects/${slug}.png`}
      alt={info.name}
      width={size}
      height={size}
      onError={() => setBroken(true)}
    />
  );
}

export function MonsterSprite({ m }: { m: CrMonster }) {
  const slug = ENEMY_ART.has(m.kind) ? m.kind : m.isBoss ? "boss" : "";
  if (!slug) return <>{m.alive ? "👹" : "💀"}</>;
  // A defeated robot stays on its square in its destroyed frame; a living one that is
  // being fought this turn plays its attack frame, otherwise it idles.
  const pose = !m.alive ? "destroyed" : m.engaged ? "attack" : "idle";
  return (
    <span
      className={`cr-enemy${m.isBoss ? " boss" : ""}${m.engaged && m.alive ? " engaged" : ""}${m.alive ? "" : " dead"}`}
      style={{ backgroundImage: `url(/sprites/crawler/enemies/${slug}/${pose}.png)` }}
      aria-label={m.kind}
    />
  );
}

// useHeroSprites drives the Bomberman-style walk animation for the board pawns in a
// turn-based game. The hero positions arrive as discrete server states, not a 60Hz
// stream, so instead of smoothed velocity it diffs each hero's global cell (tile ×3 +
// square) between states: on a change it commits a facing from the move's dominant axis
// (left/right → side, mirrored for left; up → back, down → front) and plays that pose's
// walk cycle for one step (WALK_STEP_MS), then rests on the standing frame. Returns
// spriteFor(seat).
export function useHeroSprites(v?: CrView | null): SpriteFor {
  const anim = useRef(new Map<number, { gx: number; gy: number; pose: CrPose; flip: boolean; until: number }>());
  const [, setTick] = useState(0);

  // Diff positions whenever a new server state arrives; start a walk on any mover.
  useEffect(() => {
    if (!v) return;
    const now = performance.now();
    const tilePos = new Map(v.tiles.map((t) => [t.id, [t.x, t.y] as [number, number]]));
    v.heroes.forEach((h) => {
      const tp = tilePos.get(h.tile);
      if (!tp) return;
      const gx = tp[0] * 3 + (h.square % 3);
      const gy = tp[1] * 3 + Math.floor(h.square / 3);
      const prev = anim.current.get(h.s);
      if (!prev) {
        anim.current.set(h.s, { gx, gy, pose: "front", flip: false, until: 0 });
        return;
      }
      if (prev.gx === gx && prev.gy === gy) return;
      const { pose, flip } = poseFromDelta(gx - prev.gx, gy - prev.gy, prev.flip);
      anim.current.set(h.s, { gx, gy, pose, flip, until: now + WALK_STEP_MS });
    });
  }, [v]);

  // Re-render to advance the walk frames only while a hero is mid-step (idle = quiet).
  useEffect(() => {
    const id = window.setInterval(() => {
      const now = performance.now();
      let walking = false;
      anim.current.forEach((s) => {
        if (now < s.until) walking = true;
      });
      if (walking) setTick((t) => t + 1);
    }, 90);
    return () => window.clearInterval(id);
  }, []);

  return (seat: number): { pose: CrPose; frame: number; flip: boolean } => {
    const s = anim.current.get(seat);
    if (!s) return { pose: "front", frame: 0, flip: false };
    const now = performance.now();
    const frame = now < s.until ? Math.floor(now / WALK_FRAME_MS) % POSE_LEN[s.pose] : 0;
    return { pose: s.pose, frame, flip: s.flip };
  };
}
