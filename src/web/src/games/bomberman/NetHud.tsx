import { useEffect, useRef, useState } from "react";
import type { MutableRefObject } from "react";
import type { Pace } from "./net";
import type { NetStats } from "./stats";

// The HUD samples a few times a second rather than every frame: it is a
// diagnostic, and re-rendering it at the tick rate would itself be a source of
// the stutter it is meant to measure.
const SAMPLE_MS = 400;

const STORAGE_KEY = "totem-game-stats";

/** useStatsFlag wires the F3 toggle and the ?stats=1 override together. */
export function useStatsFlag(): boolean {
  const [on, setOn] = useState(() => {
    try {
      if (new URLSearchParams(location.search).get("stats") === "1")
        return true;
      return localStorage.getItem(STORAGE_KEY) === "1";
    } catch {
      return false;
    }
  });

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.code !== "F3") return;
      e.preventDefault();
      setOn((v) => {
        try {
          localStorage.setItem(STORAGE_KEY, v ? "0" : "1");
        } catch {
          /* private mode — the toggle still works for this session */
        }
        return !v;
      });
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

  return on;
}

function Row({
  label,
  value,
  warn,
}: {
  label: string;
  value: string;
  warn?: boolean;
}) {
  return (
    <div className={`nethud-row ${warn ? "warn" : ""}`}>
      <span>{label}</span>
      <span>{value}</span>
    </div>
  );
}

export default function NetHud({
  net,
  pace,
  hz,
  predicting,
  serverBuild,
}: {
  net: MutableRefObject<NetStats>;
  pace: MutableRefObject<Pace>;
  hz: number;
  predicting: boolean;
  serverBuild: string;
}) {
  const [, redraw] = useState(0);
  const [fps, setFps] = useState(0);
  // Stalls and jumps are lifetime counters; showing them raw next to rolling
  // averages reads as a contradiction ("correction 0.000" beside "jumps 123"
  // from a burst a minute ago). Report them as a recent rate instead.
  const [rates, setRates] = useState({ stalls: 0, jumps: 0 });
  const prev = useRef({ frames: 0, stalls: 0, jumps: 0, at: 0 });

  useEffect(() => {
    prev.current = {
      frames: net.current.frames,
      stalls: net.current.stalls,
      jumps: net.current.bigCorrections,
      at: performance.now(),
    };
    const id = setInterval(() => {
      const now = performance.now();
      const m = net.current;
      const dt = (now - prev.current.at) / 1000;
      if (dt > 0) {
        setFps(Math.round((m.frames - prev.current.frames) / dt));
        setRates({
          stalls: (m.stalls - prev.current.stalls) / dt,
          jumps: (m.bigCorrections - prev.current.jumps) / dt,
        });
      }
      prev.current = {
        frames: m.frames,
        stalls: m.stalls,
        jumps: m.bigCorrections,
        at: now,
      };
      redraw((n) => n + 1);
    }, SAMPLE_MS);
    return () => clearInterval(id);
  }, [net]);

  const m = net.current;
  const gap = m.snapGap.summary();
  const rtt = m.rtt.summary();
  const corr = m.correction.summary();
  const queue = m.serverQueue.summary();
  const tickMs = 1000 / hz;

  const ms = (v: number) => `${v.toFixed(1)}ms`;
  const client = typeof __BUILD__ === "string" ? __BUILD__ : "dev";

  return (
    <div className="nethud">
      <div className="nethud-title">
        netcode · {hz}Hz {predicting ? "· predicted" : "· idle"}
      </div>
      <Row label="client" value={client} />
      <Row
        label="server"
        value={serverBuild}
        warn={serverBuild !== client && serverBuild !== "?"}
      />
      <Row label="fps" value={String(fps)} warn={fps > 0 && fps < hz * 0.8} />
      <Row
        label="ping"
        value={rtt ? `${ms(rtt.avg)} p95 ${ms(rtt.p95)}` : "–"}
        warn={!!rtt && rtt.p95 > 120}
      />
      <Row
        label="snapshot"
        // A steady stream arrives one tick apart; bunching shows up here first
        // and is the signature of a proxy or link buffering frames.
        value={gap ? `${ms(gap.avg)} p95 ${ms(gap.p95)}` : "–"}
        warn={!!gap && gap.p95 > tickMs * 2}
      />
      <Row
        label="stalls"
        value={`${rates.stalls.toFixed(1)}/s`}
        warn={rates.stalls > 2}
      />
      <Row
        label="correction"
        value={
          corr ? `${corr.avg.toFixed(3)} max ${corr.max.toFixed(3)} tiles` : "–"
        }
        // Visible corrections are the jump a player feels.
        warn={!!corr && corr.max > 0.08}
      />
      <Row
        label="jumps"
        value={`${rates.jumps.toFixed(1)}/s`}
        warn={rates.jumps > 0.5}
      />
      <Row
        label="queue"
        // Server-side depth is the client/server clock drift made visible, and
        // every queued input is a tick of added latency. The pace figure is the
        // correction being applied to the local clock to keep it shallow.
        value={
          queue
            ? `srv ${queue.avg.toFixed(1)} · local ${m.localQueue} · ${(pace.current.factor * 100).toFixed(0)}%`
            : "–"
        }
        warn={!!queue && (queue.avg < 0.3 || queue.avg > 4)}
      />
      <div className="nethud-hint">F3 to hide</div>
    </div>
  );
}
