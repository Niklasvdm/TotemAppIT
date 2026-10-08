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

const MAX_RETRIES = 6;
const backoff = (attempt: number) => Math.min(500 * 2 ** (attempt - 1), 8000);

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
  retries: number;
  send: (msg: object) => void;
}

export function useRoom<V = unknown>(code: string, name: string, meta = ""): Room<V> {
  const [status, setStatus] = useState<RoomStatus>("connecting");
  const [error, setError] = useState<string | null>(null);
  const [you, setYou] = useState(-1);
  const [host, setHost] = useState(-1);
  const [roster, setRoster] = useState<RoomRosterEntry[]>([]);
  const [view, setView] = useState<V | null>(null);
  const [retries, setRetries] = useState(0);

  const sock = useRef<WebSocket | null>(null);
  const resume = useRef("");
  const tokenKey = `totem-game-seat:${code}`;

  const send = useCallback((msg: object) => {
    const s = sock.current;
    if (s && s.readyState === WebSocket.OPEN) s.send(JSON.stringify(msg));
  }, []);

  useEffect(() => {
    let closedByUs = false;
    let attempt = 0;
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
        setRetries(0);
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
      ws.onclose = () => {
        if (closedByUs) {
          setStatus("closed");
          return;
        }
        attempt += 1;
        setRetries(attempt);
        if (attempt > MAX_RETRIES) {
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
  }, [code, name, meta, tokenKey]);

  return { status, error, you, host, roster, view, retries, send };
}
