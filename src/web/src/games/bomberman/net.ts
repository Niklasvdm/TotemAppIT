import { useCallback, useEffect, useRef, useState } from "react";
import type { MutableRefObject } from "react";
import type { Arena, Phase, RosterEntry, ServerMsg, Snapshot } from "../types";

export type ConnStatus = "connecting" | "open" | "closed";

// World holds the two most recent snapshots so the renderer can draw between
// them. Snapshots deliberately live in a ref rather than React state: at 30 a
// second, routing them through setState would re-render the tree continuously.
export interface World {
  prev: Snapshot | null;
  next: Snapshot | null;
  prevAt: number;
  nextAt: number;
  crates: string; // last crate layer seen; the server resends it only on change
}

export function newWorld(): World {
  return { prev: null, next: null, prevAt: 0, nextAt: 0, crates: "" };
}

export function socketURL(code: string, name: string, animal: string): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const q = new URLSearchParams({ code, name, animal });
  // Same origin, so the server's Origin check passes and dev goes through the
  // Vite proxy without extra configuration.
  return `${proto}//${location.host}/api/v1/games/ws?${q}`;
}

// Stats is the local player's pickup state. Snapshots carry it every tick, but
// it only ever changes when something is collected, so it is cheap to keep in
// React state for the HUD to render.
export interface Stats {
  bombs: number;
  power: number;
}

export interface Connection {
  status: ConnStatus;
  error: string | null;
  you: number;
  host: number;
  arena: Arena | null;
  roster: RosterEntry[];
  phase: Phase;
  winner: number;
  tickMs: number;
  stats: Stats;
  world: MutableRefObject<World>;
  sendInput: (dx: number, dy: number, bomb: boolean) => void;
  sendStart: () => void;
}

// useGame owns one WebSocket for the life of a joined room.
export function useGame(code: string, name: string, animal: string): Connection {
  const [status, setStatus] = useState<ConnStatus>("connecting");
  const [error, setError] = useState<string | null>(null);
  const [you, setYou] = useState(-1);
  const [host, setHost] = useState(-1);
  const [arena, setArena] = useState<Arena | null>(null);
  const [roster, setRoster] = useState<RosterEntry[]>([]);
  const [phase, setPhase] = useState<Phase>("lobby");
  const [winner, setWinner] = useState(-1);
  const [tickMs, setTickMs] = useState(1000 / 30);
  const [stats, setStats] = useState<Stats>({ bombs: 0, power: 0 });

  const world = useRef<World>(newWorld());
  const sock = useRef<WebSocket | null>(null);
  // The message handler needs our own slot, which only arrives with the
  // welcome frame — a ref keeps it available without re-subscribing.
  const youRef = useRef(-1);

  useEffect(() => {
    // live gates every callback on this socket still being the current one.
    // Closing a socket that is still CONNECTING fires error and close events,
    // and without this guard a discarded socket reports "connection failed"
    // over a healthy replacement. React 18 StrictMode remounts effects, so in
    // development that happens on every mount.
    let live = true;

    const ws = new WebSocket(socketURL(code, name, animal));
    sock.current = ws;
    world.current = newWorld();
    setStatus("connecting");
    setError(null);

    ws.onopen = () => live && setStatus("open");
    ws.onclose = () => live && setStatus("closed");
    ws.onerror = () => live && setError((e) => e ?? "connection failed");

    ws.onmessage = (ev) => {
      if (!live) return;
      let msg: ServerMsg;
      try {
        msg = JSON.parse(ev.data as string) as ServerMsg;
      } catch {
        return; // a frame we can't read is a frame we ignore
      }

      switch (msg.t) {
        case "welcome":
          youRef.current = msg.you;
          setYou(msg.you);
          setHost(msg.host);
          setArena(msg.arena);
          setRoster(msg.roster);
          setTickMs(1000 / msg.hz);
          break;

        case "roster":
          setHost(msg.host);
          setRoster(msg.roster);
          break;

        case "state": {
          const w = world.current;
          w.prev = w.next;
          w.prevAt = w.nextAt;
          w.next = msg;
          w.nextAt = performance.now();
          if (msg.c !== undefined) w.crates = msg.c;
          // Phase and winner change rarely; the identity returns let React skip
          // the re-render on every other tick.
          setPhase((p) => (p === msg.ph ? p : msg.ph));
          setWinner((x) => (x === msg.win ? x : msg.win));

          const mine = msg.p?.find((p) => p.s === youRef.current);
          if (mine) {
            setStats((s) => (s.bombs === mine.b && s.power === mine.p ? s : { bombs: mine.b, power: mine.p }));
          }
          break;
        }

        case "error":
          setError(msg.err);
          break;
      }
    };

    return () => {
      live = false;
      ws.close();
      sock.current = null;
    };
  }, [code, name, animal]);

  const sendInput = useCallback((dx: number, dy: number, bomb: boolean) => {
    const ws = sock.current;
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ t: "input", dx, dy, bomb }));
    }
  }, []);

  const sendStart = useCallback(() => {
    const ws = sock.current;
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ t: "start" }));
    }
  }, []);

  return {
    status, error, you, host, arena, roster, phase, winner, tickMs, stats,
    world, sendInput, sendStart,
  };
}

// sample reconstructs the state to draw: player positions are interpolated one
// tick behind the server, which converts arrival jitter into smooth motion
// instead of sprites snapping between tiles. Discrete things (bombs, flames,
// pickups) are taken verbatim from the newer snapshot.
export function sample(w: World, now: number, tickMs: number): Snapshot | null {
  const { prev, next } = w;
  if (!next) return null;
  if (!prev || !prev.p || !next.p) return next;

  const span = w.nextAt - w.prevAt;
  if (span <= 0) return next;

  const alpha = Math.min(1, Math.max(0, (now - tickMs - w.prevAt) / span));
  const before = new Map(prev.p.map((p) => [p.s, p]));

  return {
    ...next,
    p: next.p.map((p) => {
      const q = before.get(p.s);
      if (!q) return p;
      return { ...p, x: q.x + (p.x - q.x) * alpha, y: q.y + (p.y - q.y) * alpha };
    }),
  };
}
