// sprites.ts — totem sprite atlas for the board.
//
// A single PNG plus a JSON frame map (see src/web/public/sprites/), loaded once
// and cached. One request serves every animal's frames; 472 separate ones would
// not be an option. Players whose animal slug has frames are drawn from the
// atlas by render.ts; everyone else falls back to the emoji disc, which is not
// optional — the catalogue will never be fully covered by hand-drawn art.
//
// These are smooth cartoon drawings, not pixel art, so they are drawn with
// image smoothing ON (nearest-neighbour would make them jagged at tile size).

export type SpriteState = "front" | "back" | "side" | "defeated";

interface FrameRect {
  x: number;
  y: number;
}

interface AtlasMap {
  frameW: number;
  frameH: number;
  // Each pose is a list of frames: one entry is a static pose, several are a
  // walk cycle the game plays while the player moves.
  frames: Record<string, Partial<Record<SpriteState, FrameRect[]>>>;
}

const ATLAS_PNG = "/sprites/totems.png";
const ATLAS_JSON = "/sprites/totems.json";

let atlas: HTMLImageElement | null = null;
let map: AtlasMap | null = null;
let started = false;

// loadSprites kicks off the one-time fetch of the atlas image and frame map. It
// is safe to call every frame: it fetches at most once, and until both have
// loaded pickFrame returns null, so drawing falls back to the emoji disc.
export function loadSprites(): void {
  if (started) return;
  started = true;

  const img = new Image();
  img.onload = () => {
    atlas = img;
  };
  img.src = ATLAS_PNG;

  fetch(ATLAS_JSON)
    .then((r) => (r.ok ? (r.json() as Promise<AtlasMap>) : null))
    .then((m) => {
      if (m) map = m;
    })
    .catch(() => {
      // Leave map null; every player keeps the emoji fallback.
    });
}

// What render.ts needs to blit one frame: the atlas image, a source rect, and
// whether to mirror horizontally (the art faces right).
export interface Blit {
  img: HTMLImageElement;
  sx: number;
  sy: number;
  sw: number;
  sh: number;
  flip: boolean;
}

// Per-slot animation state. The velocity is smoothed and the facing/pose are
// sticky, so the sprite holds its heading until the player clearly turns.
interface Anim {
  lastX: number;
  lastY: number;
  vx: number; // smoothed velocity, tiles per frame
  vy: number;
  face: 1 | -1; // 1 = facing right (the art's default), -1 = facing left
  pose: SpriteState; // last committed walking pose, held while idle
}

const anims = new Map<number, Anim>();

// Over a laggy link, interpolation nudges a player's drawn position back and
// forth by sub-pixel amounts even while they walk a straight line. A bare
// "which way did x move this frame" test reads those reversals as turns and the
// sprite strobes left/right. So velocity is smoothed over several frames (EMA),
// and facing only flips on a clear, sustained push the other way.
const velSmooth = 0.3; // EMA weight for the newest frame
const moveEps = 0.02; // smoothed speed (tiles/frame) below this is "standing"
const flipEps = 0.015; // horizontal speed needed to commit a new left/right facing
const walkMs = 140; // ms per walk-cycle frame while moving

// pickFrame chooses the frame for one player and advances their animation state
// from how far they moved since the last call. `now` is the render clock, used
// to pace the walk cycle. Returns null when the atlas is not loaded or the
// animal has no sprite, which tells render.ts to fall back to the emoji disc.
export function pickFrame(
  slot: number,
  slug: string,
  x: number,
  y: number,
  alive: boolean,
  now: number,
): Blit | null {
  if (!atlas || !map) return null;
  const set = map.frames[slug];
  if (!set) return null;

  let a = anims.get(slot);
  if (!a) {
    a = { lastX: x, lastY: y, vx: 0, vy: 0, face: 1, pose: "front" };
    anims.set(slot, a);
  }
  a.vx += ((x - a.lastX) - a.vx) * velSmooth;
  a.vy += ((y - a.lastY) - a.vy) * velSmooth;
  a.lastX = x;
  a.lastY = y;

  const moving = alive && a.vx * a.vx + a.vy * a.vy > moveEps * moveEps;

  let state: SpriteState;
  if (!alive) {
    state = "defeated";
  } else if (moving) {
    // Commit a pose from the dominant axis. Horizontal keeps its facing unless
    // the sideways push is clearly the other way (flipEps) - that is what stops
    // the strobing.
    if (Math.abs(a.vx) >= Math.abs(a.vy)) {
      if (a.vx < -flipEps) a.face = -1;
      else if (a.vx > flipEps) a.face = 1;
      state = "side";
    } else {
      state = a.vy < 0 ? "back" : "front"; // up = away, down = toward camera
    }
    a.pose = state;
  } else {
    // Idle: hold the last walking pose instead of snapping to front, so a brief
    // stop between steps doesn't jerk the sprite around.
    state = a.pose;
  }

  // Fall back through to a pose the animal actually has.
  const frames =
    set[state] ?? set.front ?? set.side ?? set.back ?? set.defeated;
  if (!frames || frames.length === 0) return null;

  // Cycle frames while walking; stand on the first frame when idle or dead.
  const i = moving ? Math.floor(now / walkMs) % frames.length : 0;
  const rect = frames[i];

  return {
    img: atlas,
    sx: rect.x,
    sy: rect.y,
    sw: map.frameW,
    sh: map.frameH,
    flip: state === "side" && a.face === -1,
  };
}

// forgetSprite drops a slot's animation state, so a reused slot starts fresh.
export function forgetSprite(slot: number): void {
  anims.delete(slot);
}
