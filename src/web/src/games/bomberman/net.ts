import { useCallback, useEffect, useRef, useState } from "react";
import type { MutableRefObject } from "react";
import type {
  Arena,
  Input,
  Phase,
  PlayerDTO,
  RosterEntry,
  ServerMsg,
  Snapshot,
} from "../types";
import { makeBlocked, stepMovement, type Movable } from "./movement";
import { bigCorrection, newStats, type NetStats } from "./stats";

export type ConnStatus = "connecting" | "open" | "reconnecting" | "closed";

// Reconnect backoff. A dropped socket is usually a blip — a VPN re-keying, a
// laptop waking — and the server holds the seat for a while, so the right
// response is to walk straight back in rather than end the session.
// The tail is flat on purpose: the server holds a dropped player's seat for 30
// seconds, so the client should still be knocking when that window closes.
// Giving up sooner strands someone whose seat was waiting for them.
const retryDelays = [250, 500, 1000, 2000, 4000, 8000, 8000, 8000];

// staleAfter is how long a silent socket is given before it is presumed dead.
// A connection that dies without closing — a VPN dropping, a NAT forgetting the
// mapping, a laptop sleeping — fires no event at all, so the only evidence is
// the absence of traffic. The server sends at least once a second even in an
// idle lobby, so several seconds of nothing means the link is gone.
const staleAfter = 2500;
const watchdogEvery = 1000;

// maxUnacked bounds the replay buffer at roughly two seconds of input. If the
// server has not acknowledged anything in that long, the connection is gone and
// replaying further is pointless.
const maxUnacked = 120;

// Clock sync. The server consumes exactly one input per tick while the client
// produces exactly one per tick, so the depth of the server's queue has no
// restoring force: a burst or a stall pushes it up and it stays there forever,
// because inputs arrive precisely as fast as they drain. Every queued input is
// a tick of pure added latency, and once the queue saturates, inputs are lost.
//
// So the client steers it. The server reports its depth in every snapshot; the
// local tick interval is nudged until that depth settles at a shallow target.
// The adjustment is capped, so a bad reading can never run the clock away.
// Visual smoothing. Over a real link some correction always survives, and
// applying it straight to the drawn position reads as the character teleporting.
// The correction is applied to the simulation immediately — it is authoritative,
// and collision must use it — while the SPRITE keeps its old position and glides
// onto the new one over smoothTau. Beyond snapTiles it is not a correction but a
// genuine teleport (a respawn, a new round), which should snap.
const smoothTau = 70; // ms to decay a correction to ~37% of its size
const snapTiles = 1.25;
const maxOffset = 1.0; // never trail the true position by more than this

const queueTarget = 1.5;
const paceGain = 0.05;
const paceLimit = 0.2;

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

function clamp(v: number, limit: number): number {
  return Math.max(-limit, Math.min(limit, v));
}

export function newWorld(): World {
  return { prev: null, next: null, prevAt: 0, nextAt: 0, crates: "" };
}

// Predicted is where this client believes its own player is, which runs ahead
// of the last server snapshot by however much input is still in flight. Drawing
// the local player from here is what makes the controls feel immediate.
// Pace is the multiplier on the client's tick interval: above 1 the client
// ticks slower than the server and the queue drains, below 1 it fills.
export interface Pace {
  factor: number;
  depth: number; // smoothed server queue depth
}

export interface Predicted {
  active: boolean;
  /** Authoritative predicted position: what collision and replay use. */
  x: number;
  y: number;
  /** Visual offset, decayed toward zero every frame. Render at x+ox, y+oy. */
  ox: number;
  oy: number;
  speed: number;
}

/** decayOffset eases the drawn position onto the simulated one. */
export function decayOffset(p: Predicted, dtMs: number) {
  if (p.ox === 0 && p.oy === 0) return;
  const k = Math.exp(-dtMs / smoothTau);
  p.ox *= k;
  p.oy *= k;
  if (Math.abs(p.ox) < 1e-4) p.ox = 0;
  if (Math.abs(p.oy) < 1e-4) p.oy = 0;
}

export interface Stats {
  bombs: number;
  power: number;
}

export function socketURL(
  code: string,
  name: string,
  animal: string,
  resume = "",
): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  const q = new URLSearchParams({ code, name, animal });
  if (resume) q.set("resume", resume);
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
  serverBuild: string;
  /** How many reconnect attempts have been made since the last good frame. */
  retries: number;
  stats: Stats;
  net: MutableRefObject<NetStats>;
  pace: MutableRefObject<Pace>;
  world: MutableRefObject<World>;
  self: MutableRefObject<Predicted>;
  tickInput: (input: Input) => void;
  sendStart: () => void;
}

// useGame owns one WebSocket for the life of a joined room.
export function useGame(
  code: string,
  name: string,
  animal: string,
): Connection {
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
  const [serverBuild, setServerBuild] = useState("?");
  const [retries, setRetries] = useState(0);
  const [stats, setStats] = useState<Stats>({ bombs: 0, power: 0 });

  const net = useRef<NetStats>(newStats());
  const pace = useRef<Pace>({ factor: 1, depth: queueTarget });
  const world = useRef<World>(newWorld());
  const self = useRef<Predicted>({
    active: false,
    x: 0,
    y: 0,
    ox: 0,
    oy: 0,
    speed: 0,
  });
  // Each unacked input remembers when it was sent, which is where the RTT
  // measurement comes from: no ping/pong frame is needed, the ack already is one.
  const unacked = useRef<{ seq: number; input: Input; at: number }[]>([]);
  const seq = useRef(0);
  const sock = useRef<WebSocket | null>(null);

  // The message handler and the input tick both need these, and neither should
  // be torn down and rebuilt when they change — hence refs rather than state.
  const youRef = useRef(-1);
  // Issued by the server in every welcome; presenting it on reconnect reclaims
  // the same seat, totem and win tally. It is persisted per tab so that a page
  // refresh — or React StrictMode remounting the effect in development — walks
  // back into the existing seat instead of leaving a ghost holding it and
  // taking a fresh slot, which would fill a four-seat room after two reloads.
  const resumeToken = useRef("");
  const tokenKey = `totem-game-seat:${code}`;
  // Whether a round is actually running for us. Outside one there is nothing to
  // simulate, so feeding the server's queue would only add latency to the next.
  const playingRef = useRef(false);
  const sentIdle = useRef(false);
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
    if (acked.length)
      m.rtt.push(performance.now() - acked[acked.length - 1].at);

    unacked.current = unacked.current.filter((p) => p.seq > mine.q);
    m.localQueue = unacked.current.length;
    m.serverQueue.push(mine.d);

    // Steer the local clock toward a shallow server queue. Smoothed, because a
    // single snapshot's depth is noisy and chasing it would oscillate.
    const pc = pace.current;
    pc.depth += (mine.d - pc.depth) * 0.1;
    const drift = (pc.depth - queueTarget) * paceGain;
    pc.factor = 1 + Math.max(-paceLimit, Math.min(paceLimit, drift));

    const playable = snap.ph === "play" && mine.a;
    if (playable && !playingRef.current) {
      // A fresh round starts from an empty queue; forget the old correction.
      pace.current = { factor: 1, depth: queueTarget };
    }
    playingRef.current = playable;

    if (!playable || !pred.active) {
      // Nothing worth predicting from — a new round, a death, or the first
      // snapshot. Adopt the server's position outright.
      pred.x = mine.x;
      pred.y = mine.y;
      pred.ox = 0;
      pred.oy = 0;
      pred.active = playable;
      unacked.current = [];
      return;
    }

    const board = arenaRef.current;
    if (!board) return;
    const blocked = makeBlocked(
      board,
      world.current.crates,
      snap.b ?? [],
      youRef.current,
    );
    const replay: Movable = { x: mine.x, y: mine.y, speed: mine.v };
    for (const p of unacked.current)
      stepMovement(replay, p.input, hzRef.current, blocked);

    // How far the authoritative answer moved us. Near zero means the two rule
    // sets agree; anything visible here is the stutter a player complains about.
    const dx = replay.x - pred.x;
    const dy = replay.y - pred.y;
    const moved = Math.hypot(dx, dy);
    m.correction.push(moved);
    if (moved > bigCorrection) m.bigCorrections++;

    if (moved > snapTiles) {
      // Too far to be a correction — this is a teleport, so go there.
      pred.ox = 0;
      pred.oy = 0;
    } else {
      // Keep the sprite where it is and let it glide onto the new truth.
      pred.ox = clamp(pred.ox - dx, maxOffset);
      pred.oy = clamp(pred.oy - dy, maxOffset);
    }
    pred.x = replay.x;
    pred.y = replay.y;
  }, []);

  useEffect(() => {
    // live gates every callback on the CURRENT socket. Closing one that is
    // still CONNECTING fires error and close events, and without this guard a
    // discarded socket reports failure over a healthy replacement. React 18
    // StrictMode remounts effects, so in development that happens every mount.
    let live = true;
    let attempt = 0;
    let timer: ReturnType<typeof setTimeout> | undefined;
    // A refusal from the server (room full, unknown code) is final; retrying it
    // would just be refused again.
    let terminal = false;
    let lastFrameAt = performance.now();
    // Retires whichever socket is current; reassigned by each connect().
    let retire: () => void = () => {};

    // Nothing else notices a socket that stops delivering without closing. The
    // server sends at least once a second even in an idle lobby, so a long
    // silence means the link is gone whatever the socket claims its state is.
    const watchdog = setInterval(() => {
      if (!live) return;
      if (performance.now() - lastFrameAt < staleAfter) return;
      retire();
    }, watchdogEvery);

    try {
      resumeToken.current =
        sessionStorage.getItem(tokenKey) || resumeToken.current;
    } catch {
      /* private mode — reconnects still work within this page's lifetime */
    }

    const connect = () => {
      if (!live) return;

      const ws = new WebSocket(
        socketURL(code, name, animal, resumeToken.current),
      );
      sock.current = ws;
      // Simulation state is rebuilt from the first snapshot after the handshake,
      // so none of it should survive a reconnect.
      world.current = newWorld();
      net.current = newStats();
      pace.current = { factor: 1, depth: queueTarget };
      playingRef.current = false;
      sentIdle.current = false;
      self.current = { active: false, x: 0, y: 0, ox: 0, oy: 0, speed: 0 };
      unacked.current = [];
      seq.current = 0;
      setStatus(attempt === 0 ? "connecting" : "reconnecting");

      ws.onopen = () => {
        if (!live) return;
        lastFrameAt = performance.now();
        attempt = 0;
        setRetries(0);
        setStatus("open");
        setError(null);
      };

      // giveUpOn retires THIS socket and queues the next attempt. It is driven
      // from two places — the socket closing, and the watchdog deciding it has
      // gone quiet — because neither is reliable alone: closing a socket whose
      // link is already dead leaves it in CLOSING forever and never fires
      // onclose at all. Whichever gets there first wins; `done` ignores the other.
      let done = false;
      const giveUpOn = () => {
        if (!live || done) return;
        done = true;
        try {
          ws.close();
        } catch {
          /* already going */
        }
        if (terminal || attempt >= retryDelays.length) {
          setStatus("closed");
          return;
        }
        // The seat is held server-side for a grace period, so keep trying; the
        // resume token walks us back into the same slot, score and all.
        const wait = retryDelays[attempt];
        attempt += 1;
        setRetries(attempt);
        setStatus("reconnecting");
        timer = setTimeout(connect, wait);
      };

      retire = giveUpOn;
      ws.onclose = giveUpOn;

      // Deliberately silent: a socket error is always followed by a close or
      // caught by the watchdog, and surfacing it would replace the reconnecting
      // state with a dead end on the very first retry.
      ws.onerror = () => {};

      ws.onmessage = (ev) => {
        if (!live) return;
        // Any frame at all proves the link is alive; the watchdog reads this.
        lastFrameAt = performance.now();
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
            setServerBuild(msg.build || "?");
            if (msg.token) {
              resumeToken.current = msg.token;
              try {
                sessionStorage.setItem(tokenKey, msg.token);
              } catch {
                /* private mode */
              }
            }
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
                s.bombs === mine.b && s.power === mine.p
                  ? s
                  : { bombs: mine.b, power: mine.p },
              );
            }
            break;
          }

          case "error":
            setError(msg.err);
            break;
        }
      };
    };

    connect();
    return () => {
      live = false;
      clearTimeout(timer);
      clearInterval(watchdog);
      // Say goodbye so the server releases the seat rather than holding it for
      // a reconnect that is never coming. Without this, navigating away or
      // refreshing leaves a ghost occupying a slot until its grace period
      // lapses — four reloads would fill the room with nobody.
      const ws = sock.current;
      if (ws?.readyState === WebSocket.OPEN) {
        try {
          ws.send(JSON.stringify({ t: "bye" }));
        } catch {
          /* already going away */
        }
      }
      ws?.close();
      sock.current = null;
    };
  }, [code, name, animal, tokenKey, reconcile]);

  // tickInput is called once per client tick with whatever is held down. It
  // sends the input, keeps a copy for replay, and applies it locally straight
  // away — that local application is the whole point: the player sees their
  // own move on the next frame instead of a round trip later.
  const tickInput = useCallback((input: Input) => {
    const ws = sock.current;
    const open = ws?.readyState === WebSocket.OPEN;

    if (!playingRef.current) {
      // Between rounds, in the lobby, or dead. Send one neutral input on the
      // way out — the server holds the last one it consumed when its queue runs
      // dry, so a stale direction would keep walking us — then go quiet.
      if (sentIdle.current) return;
      sentIdle.current = true;
      unacked.current = [];
      if (open) {
        seq.current += 1;
        ws!.send(
          JSON.stringify({
            t: "input",
            seq: seq.current,
            dx: 0,
            dy: 0,
            bomb: false,
          }),
        );
      }
      return;
    }
    sentIdle.current = false;

    seq.current += 1;
    const n = seq.current;
    if (open) {
      ws!.send(
        JSON.stringify({
          t: "input",
          seq: n,
          dx: input.dx,
          dy: input.dy,
          bomb: input.bomb,
        }),
      );
    }

    const pred = self.current;
    const board = arenaRef.current;
    if (!pred.active || !board) return;

    unacked.current.push({ seq: n, input, at: performance.now() });
    if (unacked.current.length > maxUnacked) unacked.current.shift();

    const blocked = makeBlocked(
      board,
      world.current.crates,
      world.current.next?.b ?? [],
      youRef.current,
    );
    stepMovement(pred, input, hzRef.current, blocked);
  }, []);

  const sendStart = useCallback(() => {
    const ws = sock.current;
    if (ws?.readyState === WebSocket.OPEN)
      ws.send(JSON.stringify({ t: "start" }));
  }, []);

  return {
    status,
    error,
    you,
    host,
    arena,
    roster,
    phase,
    winner,
    tickMs,
    fuseTicks,
    serverBuild,
    retries,
    stats,
    net,
    pace,
    world,
    self,
    tickInput,
    sendStart,
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
      return {
        ...p,
        x: q.x + (p.x - q.x) * alpha,
        y: q.y + (p.y - q.y) * alpha,
      };
    }),
  };
}
