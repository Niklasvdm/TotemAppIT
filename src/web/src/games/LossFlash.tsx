import { useEffect, useRef, useState } from "react";

// useLossPulse returns true for a short moment whenever `value` DECREASES — the
// cue for a "you just lost a life / fuse / health" animation. Pass the current
// count (lives, fuses, health, …); it ignores increases and the initial value.
export function useLossPulse(value: number | undefined, ms = 1300): boolean {
  const prev = useRef<number | null>(null);
  const [on, setOn] = useState(false);
  useEffect(() => {
    if (value == null) return;
    const p = prev.current;
    prev.current = value;
    if (p != null && value < p) {
      setOn(true);
      const h = setTimeout(() => setOn(false), ms);
      return () => clearTimeout(h);
    }
  }, [value, ms]);
  return on;
}

// LossFlash pops a big icon in the centre of the screen and fades it out, shared
// by every game so a lost life/fuse/health reads the same way everywhere.
export function LossFlash({ show, icon, label = "lost" }: { show: boolean; icon: string; label?: string }) {
  if (!show) return null;
  return (
    <div className="loss-flash" role="status" aria-label={label}>
      <span>{icon}</span>
    </div>
  );
}
