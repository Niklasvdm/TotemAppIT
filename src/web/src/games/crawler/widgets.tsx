// Tiny shared UI atoms for the crawler.

// InfoDot is a small ⓘ badge with a hover box explaining an ability.
export function InfoDot({ text }: { text: string }) {
  return (
    <span className="cr-info" tabIndex={0}>
      ⓘ<span className="cr-info-box">{text}</span>
    </span>
  );
}
