// Tiny shared UI atoms for the crawler.
import { useRef, useState } from "react";

// InfoDot is a small ⓘ badge with a hover box explaining an ability. The box is positioned
// with `position: fixed` (coordinates measured from the dot on hover) so it escapes the left
// rail's `overflow-y: auto` clip — an absolutely-positioned box would be cut off there.
export function InfoDot({ text }: { text: string }) {
  const ref = useRef<HTMLSpanElement>(null);
  const [pos, setPos] = useState<{ left: number; bottom: number } | null>(null);
  const show = () => {
    const r = ref.current?.getBoundingClientRect();
    if (r) setPos({ left: r.left + r.width / 2, bottom: window.innerHeight - r.top + 6 });
  };
  const hide = () => setPos(null);
  return (
    <span ref={ref} className="cr-info" tabIndex={0} onMouseEnter={show} onMouseLeave={hide} onFocus={show} onBlur={hide}>
      ⓘ
      {pos && <span className="cr-info-box" style={{ left: pos.left, bottom: pos.bottom }}>{text}</span>}
    </span>
  );
}
