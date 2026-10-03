import { useEffect, useState } from "react";
import type { MutableRefObject } from "react";
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
      if (new URLSearchParams(location.search).get("stats") === "1") return true;
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

function Row({ label, value, warn }: { label: string; value: string; warn?: boolean }) {
  return (
    <div className={`nethud-row ${warn ? "warn" : ""}`}>
      <span>{label}</span>
      <span>{value}</span>
    </div>
  );
}

export default function NetHud({
  net,
  hz,
  predicting,
}: {
  net: MutableRefObject<NetStats>;
  hz: number;
  predicting: boolean;
}) {
  const [, tick] = useState(0);
  const [fps, setFps] = useState(0);

  useEffect(() => {
    let lastFrames = net.current.frames;
    let lastAt = performance.now();
    const id = setInterval(() => {
      const now = performance.now();
      const m = net.current;
      setFps(Math.round(((m.frames - lastFrames) * 1000) / (now - lastAt)));
      lastFrames = m.frames;
      lastAt = now;
      tick((n) => n + 1);
    }, SAMPLE_MS);
    return () => clearInterval(id);
  }, [net]);

  const m = net.current;
  const gap = m.snapGap.summary();
  const rtt = m.rtt.summary();
  const corr = m.correction.summary();
  const queue = m.serverQueue.summary();
  const tickMs = 1000 / hz;

  const ms = (v: number | undefined) => (v === undefined ? "–" : `${v.toFixed(1)}ms`);

  return (
    <div className="nethud">
      <div className="nethud-title">
        netcode · {hz}Hz {predicting ? "· predicted" : "· server-only"}
      </div>
      <Row label="fps" value={String(fps)} warn={fps > 0 && fps < hz * 0.8} />
      <Row
        label="ping"
        value={rtt ? `${ms(rtt.avg)} p95 ${ms(rtt.p95)}` : "–"}
        warn={!!rtt && rtt.p95 > 120}
      />
      <Row
        label="snapshot"
        value={gap ? `${ms(gap.avg)} p95 ${ms(gap.p95)}` : "–"}
        // A steady stream arrives one tick apart; bunching shows up here first
        // and is the signature of a proxy buffering frames.
        warn={!!gap && gap.p95 > tickMs * 2}
      />
      <Row label="stalls" value={String(m.stalls)} warn={m.stalls > 0} />
      <Row
        label="correction"
        value={corr ? `${corr.avg.toFixed(3)} max ${corr.max.toFixed(3)} tiles` : "–"}
        // Visible corrections are the jump a player feels.
        warn={!!corr && corr.max > 0.08}
      />
      <Row label="jumps" value={String(m.bigCorrections)} warn={m.bigCorrections > 0} />
      <Row
        label="queue"
        // Server-side depth is the client/server clock drift made visible: it
        // should hover near 1. Pinned at 0 means the server is starving and
        // repeating inputs; pinned high means we are running fast.
        value={queue ? `srv ${queue.avg.toFixed(1)} · local ${m.localQueue}` : "–"}
        warn={!!queue && (queue.avg < 0.35 || queue.avg > 4)}
      />
      <div className="nethud-hint">F3 to hide</div>
    </div>
  );
}
