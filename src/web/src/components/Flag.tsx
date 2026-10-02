// Inline SVG flags. Flag *emoji* (🇮🇹🇬🇧) don't render on Windows and some
// older Androids — they fall back to "IT"/"GB" letters — so we draw them.
type Code = "it" | "gb" | "be" | "nl";

function Svg({ children, title }: { children: React.ReactNode; title: string }) {
  return (
    <svg className="flag-svg" viewBox="0 0 60 30" role="img" aria-label={title}>
      <title>{title}</title>
      {children}
    </svg>
  );
}

export default function Flag({ code, title }: { code: Code; title?: string }) {
  switch (code) {
    case "it":
      return (
        <Svg title={title ?? "Italiano"}>
          <rect width="20" height="30" fill="#009246" />
          <rect x="20" width="20" height="30" fill="#fff" />
          <rect x="40" width="20" height="30" fill="#ce2b37" />
        </Svg>
      );
    case "be":
      return (
        <Svg title={title ?? "Belgisch Nederlands"}>
          <rect width="20" height="30" fill="#000" />
          <rect x="20" width="20" height="30" fill="#fdda24" />
          <rect x="40" width="20" height="30" fill="#ef3340" />
        </Svg>
      );
    case "nl":
      return (
        <Svg title={title ?? "Nederlands"}>
          <rect width="60" height="10" fill="#ae1c28" />
          <rect y="10" width="60" height="10" fill="#fff" />
          <rect y="20" width="60" height="10" fill="#21468b" />
        </Svg>
      );
    case "gb":
      return (
        <Svg title={title ?? "English"}>
          <rect width="60" height="30" fill="#012169" />
          <path d="M0 0 60 30M60 0 0 30" stroke="#fff" strokeWidth="6" />
          <path d="M0 0 60 30M60 0 0 30" stroke="#c8102e" strokeWidth="4" />
          <path d="M30 0v30M0 15h60" stroke="#fff" strokeWidth="10" />
          <path d="M30 0v30M0 15h60" stroke="#c8102e" strokeWidth="6" />
        </Svg>
      );
  }
}
