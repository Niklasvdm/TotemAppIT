// Mirror of internal/game/protocol.go. Field names are single letters because
// snapshots cross the wire 30 times a second; keep both sides in step.

export type Phase = "lobby" | "play" | "over";
export type PowerKind = "bomb" | "fire" | "speed";

export interface Arena {
  w: number;
  h: number;
  solid: string; // row-major, '#' wall / '.' open
}

export interface RosterEntry {
  s: number; // slot
  n: string; // name
  m: string; // animal slug
  w: number; // rounds won
}

export interface PlayerDTO {
  s: number;
  x: number;
  y: number;
  a: boolean; // alive
  b: number; // bomb stock
  p: number; // blast radius
  q: number; // last input sequence folded into this position
  v: number; // movement speed, tiles per second
  d: number; // inputs still queued on the server for this player
}

// Input is one client tick's intent. Mirrors game.Input in Go.
export interface Input {
  dx: number;
  dy: number;
  bomb: boolean;
}

export interface BombDTO {
  x: number;
  y: number;
  f: number; // fuse ticks remaining
  s: number; // bitmask of slots still allowed to step off this bomb
}

export interface FlameDTO {
  x: number;
  y: number;
}

export interface PowerDTO {
  x: number;
  y: number;
  k: PowerKind;
}

export interface Snapshot {
  t: "state";
  k: number; // tick
  ph: Phase;
  win: number; // winning slot, -1 for nobody
  p?: PlayerDTO[];
  b?: BombDTO[];
  f?: FlameDTO[];
  u?: PowerDTO[];
  c?: string; // crate layer, present only when it changed
}

export interface WelcomeMsg {
  t: "welcome";
  you: number;
  code: string;
  host: number;
  hz: number;
  fuse: number; // bomb fuse length in ticks
  build: string; // server build stamp
  arena: Arena;
  roster: RosterEntry[];
}

export interface RosterMsg {
  t: "roster";
  host: number;
  roster: RosterEntry[];
}

export interface ErrorMsg {
  t: "error";
  err: string;
}

export type ServerMsg = Snapshot | WelcomeMsg | RosterMsg | ErrorMsg;

export const MAX_PLAYERS = 4;

// Per-slot colours, chosen to stay legible on both the cream and dark themes.
export const SLOT_COLORS = ["#2e9e6b", "#d1495b", "#e8b54b", "#4b8fe8"];

export const POWER_ICONS: Record<PowerKind, string> = {
  bomb: "💣",
  fire: "🔥",
  speed: "👟",
};
