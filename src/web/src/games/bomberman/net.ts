import { useCallback, useEffect, useRef, useState } from "react";
import type { MutableRefObject } from "react";
import type { Arena, Input, Phase, PlayerDTO, RosterEntry, ServerMsg, Snapshot } from "../types";
import { makeBlocked, stepMovement, type Movable } from "./movement";
import { bigCorrection, newStats, type NetStats } from "./stats";

export type ConnStatus = "connecting" | "open" | "closed";

// maxUnacked bounds the replay buffer at roughly two seconds of input. If the
// server has not acknowledged anything in that long, the connection is gone and
// replaying further is pointless.
const maxUnacked = 120;

// World holds the two most recent snapshots so the renderer can draw between
// them. Snapshots deliberately live in a ref rather than React state: at the
// tick rate, routing them through setState would re-render the tree endlessly.
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

// Predicted is where this client believes its own player is, which runs ahead
// of the last server snapshot by however much input is still in flight. Drawing
// the local player from here is what makes the controls feel immediate.
export interface Predicted {
  active: boolean;
  x: number;
  y: number;
  speed: number;
}

export interface Stats {
  bombs: number;
  power: number;
}

export function socketURL(code: string, name: string, animal: string): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const q = new URLSearchParams({ code, name, animal });
  // Same origin, so the server's Origin check passes and dev goes through the
  // Vite proxy without extra configuration.
  return `${proto}//${location.host}/api/v1/games/ws?${q}`;
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
  fuseTicks: number;
  stats: Stats;
  net: MutableRefObject<NetStats>;
  world: MutableRefObject<World>;
  self: MutableRefObject<Predicted>;
  tickInput: (input: Input) => void;
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
  const [tickMs, setTickMs] = useState(1000 / 60);
  const [fuseTicks, setFuseTicks] = useState(120);
  const [stats, setStats] = useState<Stats>({ bombs: 0, power: 0 });

  const net = useRef<NetStats>(newStats());
  const world = useRef<World>(newWorld());
  const self = useRef<Predicted>({ active: false, x: 0, y: 0, speed: 0 });
  // Each unacked input remembers when it was sent, which is where the RTT
  // measurement comes from: no ping/pong frame is needed, the ack already is one.
  const unacked = useRef<{ seq: number; input: Input; at: number }[]>([]);
  const seq = useRef(0);
  const sock = useRef<WebSocket | null>(null);

  // The message handler and the input tick both need these, and neither should
  // be torn down and rebuilt when they change — hence refs rather than state.
  const youRef = useRef(-1);
  const hzRef = useRef(60);
  const arenaRef = useRef<Arena | null>(null);

  // reconcile folds in an authoritative position: rewind to what the server
  // says, then re-apply the inputs it has not seen yet. Those inputs WILL be
  // applied, so replaying them is not guesswork — it is the same arithmetic the
  // server is about to do, run early.
  const reconcile = useCallback((mine: PlayerDTO, snap: Snapshot) => {
    const pred = self.current;
    const m = net.current;
    pred.speed = mine.v;

    // The newest input this snapshot acknowledges tells us the round trip.
    const acked = unacked.current.filter((p) => p.seq <= mine.q);
    if (acked.length) m.rtt.push(performance.now() - acked[acked.length - 1].at);

    unacked.current = unacked.current.filter((p) => p.seq > mine.q);
    m.localQueue = unacked.current.length;
    m.serverQueue.push(mine.d);

    const playable = snap.ph === "play" && mine.a;
    if (!playable || !pred.active) {
      // Nothing worth predicting from — a new round, a death, or the first
      // snapshot. Adopt the server's position outright.
      pred.x = mine.x;
      pred.y = mine.y;
      pred.active = playable;
      unacked.current = [];
      return;
    }

    const board = arenaRef.current;
    if (!board) return;
    const blocked = makeBlocked(board, world.current.crates, snap.b ?? [], youRef.current);
    const replay: Movable = { x: mine.x, y: mine.y, speed: mine.v };
    for (const p of unacked.current) stepMovement(replay, p.input, hzRef.current, blocked);

    // How far the authoritative answer moved us. Near zero means the two rule
    // sets agree; anything visible here is the stutter a player complains about.
    const moved = Math.hypot(replay.x - pred.x, replay.y - pred.y);
    m.correction.push(moved);
    if (moved > bigCorrection) m.bigCorrections++;

    pred.x = replay.x;
    pred.y = replay.y;
  }, []);

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
    net.current = newStats();
    self.current = { active: false, x: 0, y: 0, speed: 0 };
    unacked.current = [];
    seq.current = 0;
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
          hzRef.current = msg.hz;
          arenaRef.current = msg.arena;
          setYou(msg.you);
          setHost(msg.host);
          setArena(msg.arena);
          setRoster(msg.roster);
          setTickMs(1000 / msg.hz);
          setFuseTicks(msg.fuse);
          break;

        case "roster":
          setHost(msg.host);
          setRoster(msg.roster);
          break;

        case "state": {
          const w = world.current;
          const at = performance.now();
          if (w.nextAt) {
            const gap = at - w.nextAt;
            net.current.snapGap.push(gap);
            if (gap > (1000 / hzRef.current) * 2) net.current.stalls++;
          }
          w.prev = w.next;
          w.prevAt = w.nextAt;
          w.next = msg;
          w.nextAt = at;
          if (msg.c !== undefined) w.crates = msg.c;

          // Phase and winner change rarely; the identity returns let React skip
          // the re-render on every other tick.
          setPhase((p) => (p === msg.ph ? p : msg.ph));
          setWinner((x) => (x === msg.win ? x : msg.win));

          const mine = msg.p?.find((p) => p.s === youRef.current);
          if (mine) {
            reconcile(mine, msg);
            setStats((s) =>
              s.bombs === mine.b && s.power === mine.p ? s : { bombs: mine.b, power: mine.p },
            );
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
  }, [code, name, animal, reconcile]);

  // tickInput is called once per client tick with whatever is held down. It
  // sends the input, keeps a copy for replay, and applies it locally straight
  // away — that local application is the whole point: the player sees their
  // own move on the next frame instead of a round trip later.
  const tickInput = useCallback((input: Input) => {
    seq.current += 1;
    const n = seq.current;

    const ws = sock.current;
    if (ws?.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({ t: "input", seq: n, dx: input.dx, dy: input.dy, bomb: input.bomb }));
    }

    const pred = self.current;
    const board = arenaRef.current;
    if (!pred.active || !board) return;

    unacked.current.push({ seq: n, input, at: performance.now() });
    if (unacked.current.length > maxUnacked) unacked.current.shift();

    const blocked = makeBlocked(board, world.current.crates, world.current.next?.b ?? [], youRef.current);
    stepMovement(pred, input, hzRef.current, blocked);
  }, []);

  const sendStart = useCallback(() => {
    const ws = sock.current;
    if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ t: "start" }));
  }, []);

  return {
    status, error, you, host, arena, roster, phase, winner, tickMs, fuseTicks, stats,
    net, world, self, tickInput, sendStart,
  };
}

// sample reconstructs the state to draw. Other players are interpolated one
// tick behind the server, which converts arrival jitter into smooth motion;
// the local player is NOT drawn from here (see Predicted). Discrete things —
// bombs, flames, pickups — are taken verbatim from the newer snapshot.
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
