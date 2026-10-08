import type { CSSProperties } from "react";

// A tiny card face for the number games — coloured by its tens band (0–9, 10–19,
// …, 100) with the number printed. Pure CSS, no image assets.
const BANDS = [
  "#5b8def", // 0–9
  "#2e9e6b", // 10–19
  "#e0a33d", // 20–29
  "#e0683f", // 30–39
  "#d1495b", // 40–49
  "#b5529e", // 50–59
  "#7b68ee", // 60–69
  "#1aa6b7", // 70–79
  "#a9712f", // 80–89
  "#3a7d44", // 90–99
  "#2c3e50", // 100
];

export function cardColor(n: number): string {
  return BANDS[Math.min(BANDS.length - 1, Math.max(0, Math.floor(n / 10)))];
}

export function Card({
  n,
  className = "",
  onClick,
  disabled,
  title,
  style,
}: {
  n: number;
  className?: string;
  onClick?: () => void;
  disabled?: boolean;
  title?: string;
  style?: CSSProperties;
}) {
  const s: CSSProperties = { background: cardColor(n), ...style };
  if (onClick) {
    return (
      <button type="button" className={`gcard ${className}`} style={s} onClick={onClick} disabled={disabled} title={title}>
        {n}
      </button>
    );
  }
  return (
    <span className={`gcard ${className}`} style={s} title={title}>
      {n}
    </span>
  );
}

// lastN returns the final n items — for capping a long pile history on screen.
export function lastN<T>(arr: T[], n: number): T[] {
  return arr.length > n ? arr.slice(arr.length - n) : arr;
}
