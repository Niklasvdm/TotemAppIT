import { useCallback, useEffect, useRef, useState } from "react";

// useRoom is the game-agnostic WebSocket transport: it opens one socket for a
// joined room, handles the welcome, keeps the roster, resumes its seat after a
// drop, and hands the game its latest view. Turn-based games (Codenames, …) pair
// it with their own view type and render whatever the server last sent — no
// prediction, unlike Bomberman's bespoke net.ts.

export type RoomStatus = "connecting" | "open" | "reconnecting" | "closed";

export interface RoomRosterEntry {
  s: number; // seat
  n: string; // name
  m?: string; // per-game meta (totem slug, …)
  gone?: boolean;
}

// Keep retrying for this long before giving up, rather than after a fixed NUMBER of tries.
// It is comfortably longer than the server's seat-hold grace (reconnectGrace), so a network
// blip never makes the client abandon a seat the server would still take back. (The old
// 6-try cap gave up after ~23s, far short of that grace — a client stranded mid-outage.)
const RECONNECT_WINDOW_MS = 5 * 60 * 1000;

// Full-jitter exponential backoff (the AWS-blessed standard): the ceiling doubles each
// attempt up to a cap, and the actual wait is a random point in [0, ceiling]. The doubling
// backs off a struggling server; the jitter stops many clients that dropped together (e.g.
// on a server restart) from retrying in lockstep and stampeding it.
const BACKOFF_BASE_MS = 500;
const BACKOFF_CAP_MS = 30_000;
const backoff = (attempt: number) => {
  const ceiling = Math.min(BACKOFF_CAP_MS, BACKOFF_BASE_MS * 2 ** (attempt - 1));
  return Math.random() * ceiling;
};

// closeReasonText turns a WebSocket close into a human sentence: a server-sent reason wins;
// otherwise the code is mapped (a plain network drop is 1006 with no reason, which is the
// most the browser can tell us).
function closeReasonText(ev: CloseEvent): string {
  if (ev.reason) return ev.reason;
  switch (ev.code) {
    case 1001:
      return "The server is restarting.";
    case 1011:
      return "The server hit an error.";
    case 1005:
    case 1006:
      return "Connection lost — your network dropped, or the server restarted.";
    default:
      return `Disconnected (code ${ev.code}).`;
  }
}

// The server closes with this application code (browsers expose 4000-4999 on the close
// event) when the room no longer exists or has closed. It means "stop reconnecting":
// a transient drop surfaces as 1006, so without this a phone that re-opens a dead room
// would retry it six times over before giving up.
const WS_CLOSE_ROOM_GONE = 4404;

function socketURL(code: string, name: string, meta: string, resume: string): string {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  // The server reads the meta channel as the "animal" param (Bomberman's totem);
  // other games pass whatever they need, or "".
  const q = new URLSearchParams({ code, name, animal: meta });
  if (resume) q.set("resume", resume);
  return `${proto}//${location.host}/api/v1/games/ws?${q}`;
}

export interface Room<V> {
  status: RoomStatus;
  error: string | null;
  you: number; // your seat, -1 until the welcome
  host: number;
  roster: RoomRosterEntry[];
  view: V | null; // the latest game "state" frame
  game: string; // the room's game slug from the welcome ("" until then); a page compares it to its own
  retries: number;
  closeReason: string | null; // why the socket last closed (shown while reconnecting / disconnected)
  reconnect: () => void; // manually (re)start the connection, resuming the held seat
  send: (msg: object) => void;
}

// seatKey distinguishes several LOCAL players sharing one device (local multiplayer):
// each opens its own socket/seat to the same room, so their reconnect tokens must not
// collide in sessionStorage. Single-player callers leave it "" (one token per room).
export function useRoom<V = unknown>(code: string, name: string, meta = "", seatKey = ""): Room<V> {
  const [status, setStatus] = useState<RoomStatus>("connecting");
  const [error, setError] = useState<string | null>(null);
  const [you, setYou] = useState(-1);
  const [host, setHost] = useState(-1);
  const [roster, setRoster] = useState<RoomRosterEntry[]>([]);
  const [view, setView] = useState<V | null>(null);
  const [game, setGame] = useState("");
  const [retries, setRetries] = useState(0);
  const [closeReason, setCloseReason] = useState<string | null>(null);
  const [nonce, setNonce] = useState(0); // bumped by reconnect() to re-run the connect effect

  const sock = useRef<WebSocket | null>(null);
  const resume = useRef("");
  const tokenKey = `totem-game-seat:${code}${seatKey ? ":" + seatKey : ""}`;

  const send = useCallback((msg: object) => {
    const s = sock.current;
    if (s && s.readyState === WebSocket.OPEN) s.send(JSON.stringify(msg));
  }, []);

  // reconnect restarts the connection (resuming the held seat via the stored token). Used
  // by the Disconnected screen's Reconnect button, and to retry sooner than the backoff.
  const reconnect = useCallback(() => setNonce((n) => n + 1), []);

  useEffect(() => {
    let closedByUs = false;
    let attempt = 0;
    let firstFailAt = 0; // when the current run of failures began (0 = connected / none yet)
    let timer: ReturnType<typeof setTimeout> | undefined;
    try {
      resume.current = sessionStorage.getItem(tokenKey) || resume.current;
    } catch {
      /* private mode: reconnects still work within this page's life */
    }

    const connect = () => {
      setStatus(attempt === 0 ? "connecting" : "reconnecting");
      const ws = new WebSocket(socketURL(code, name, meta, resume.current));
      sock.current = ws;

      ws.onopen = () => {
        attempt = 0;
        firstFailAt = 0;
        setRetries(0);
        setCloseReason(null);
        setStatus("open");
      };
      ws.onmessage = (ev) => {
        let m: { t?: string; [k: string]: unknown };
        try {
          m = JSON.parse(ev.data as string);
        } catch {
          return;
        }
        switch (m.t) {
          case "welcome":
            setYou(m.you as number);
            setHost(m.host as number);
            if (typeof m.game === "string") setGame(m.game);
            setRoster((m.roster as RoomRosterEntry[]) ?? []);
            if (typeof m.token === "string" && m.token) {
              resume.current = m.token;
              try {
                sessionStorage.setItem(tokenKey, m.token);
              } catch {
                /* ignore */
              }
            }
            break;
          case "roster":
            setHost(m.host as number);
            setRoster((m.roster as RoomRosterEntry[]) ?? []);
            break;
          case "error":
            setError((m.err as string) ?? "error");
            break;
          default:
            // Any other frame is a game view (Codenames sends t:"state").
            setView(m as V);
        }
      };
      ws.onclose = (ev) => {
        if (closedByUs) {
          setStatus("closed");
          return;
        }
        if (ev.code === WS_CLOSE_ROOM_GONE) {
          // The room is gone for good: don't reconnect, and drop the stale resume token
          // so a later join (even to a recreated code) starts clean.
          try {
            sessionStorage.removeItem(tokenKey);
          } catch {
            /* ignore */
          }
          resume.current = "";
          setError("This room is no longer available.");
          setCloseReason("This room is no longer available.");
          setStatus("closed");
          return;
        }
        setCloseReason(closeReasonText(ev));
        if (firstFailAt === 0) firstFailAt = Date.now();
        attempt += 1;
        setRetries(attempt);
        // Keep retrying for the whole window (not a fixed count), so an outage shorter than
        // the server's grace is always ridden out instead of abandoning a live seat.
        if (Date.now() - firstFailAt > RECONNECT_WINDOW_MS) {
          setStatus("closed");
          return;
        }
        setStatus("reconnecting");
        timer = setTimeout(connect, backoff(attempt));
      };
      ws.onerror = () => {
        /* onclose follows */
      };
    };
    connect();

    return () => {
      closedByUs = true;
      if (timer) clearTimeout(timer);
      sock.current?.close();
    };
  }, [code, name, meta, tokenKey, nonce]);

  return { status, error, you, host, roster, view, game, retries, closeReason, reconnect, send };
}
