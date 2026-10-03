// Rolling netcode measurements for the dev HUD.
//
// Everything here is collected on the hot path, so it must stay allocation-free
// per sample: each series is a fixed ring buffer that is only summarised when
// the HUD reads it (a few times a second), never per frame.

const WINDOW = 180; // ~3 seconds at 60Hz

export class Series {
  private buf = new Float64Array(WINDOW);
  private n = 0;
  private i = 0;

  push(v: number) {
    this.buf[this.i] = v;
    this.i = (this.i + 1) % WINDOW;
    if (this.n < WINDOW) this.n++;
  }

  get count() {
    return this.n;
  }

  /** summary returns avg, p95 and max over the window, or null when empty. */
  summary(): { avg: number; p95: number; max: number } | null {
    if (this.n === 0) return null;
    const v = Array.from(this.buf.subarray(0, this.n)).sort((a, b) => a - b);
    let sum = 0;
    for (const x of v) sum += x;
    return {
      avg: sum / v.length,
      p95: v[Math.min(v.length - 1, Math.floor(v.length * 0.95))],
      max: v[v.length - 1],
    };
  }
}

export interface NetStats {
  /** Wall time between arriving snapshots, in ms. Should sit at one tick. */
  snapGap: Series;
  /** Round trip: input sent -> the snapshot that acknowledges it, in ms. */
  rtt: Series;
  /** How far reconciliation moved the predicted player, in tiles. */
  correction: Series;
  /** Inputs still queued on the server for us. Should hover near 1. */
  serverQueue: Series;
  /** Inputs we have sent but the server has not acknowledged. */
  localQueue: number;
  /** Corrections large enough to be visible as a jump. */
  bigCorrections: number;
  /** Snapshots that arrived more than two ticks after the previous one. */
  stalls: number;
  /** Frames rendered, sampled by the HUD to derive FPS. */
  frames: number;
}

export function newStats(): NetStats {
  return {
    snapGap: new Series(),
    rtt: new Series(),
    correction: new Series(),
    serverQueue: new Series(),
    localQueue: 0,
    bigCorrections: 0,
    stalls: 0,
    frames: 0,
  };
}

// bigCorrection is the threshold, in tiles, above which a reconciliation is
// assumed to be visible as a jump rather than absorbed as sub-pixel drift.
export const bigCorrection = 0.08;
