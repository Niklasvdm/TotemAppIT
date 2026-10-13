import { Link } from "react-router-dom";

// A room is created for one game (the server binds the slug at creation) and the
// welcome frame now carries that slug. If a client page for game X ends up joined to a
// room created for game Y (a shared/mistyped code), rendering Y's state frames in X's
// UI produces garbage. Each page compares the welcome's `game` to its own slug and, on
// a mismatch, shows this notice instead.
//
// SECURITY: the slug comes from the server and is treated as untrusted. It is only ever
// looked up in this fixed allowlist; an unknown slug yields no link and no navigation.
// The only navigation target is a hardcoded in-app route from the table below, with the
// already-validated room code as a query param. We never build a path or any other
// string out of the raw slug.

interface GameInfo {
  route: string; // the in-app route segment under /games/
  name: string; // human label
}

// Keyed by the server game slug (the gameFactories keys in internal/game/engine.go).
// Route segments match the <Route> paths in App.tsx (four differ from the slug).
const GAMES: Record<string, GameInfo> = {
  bomberman: { route: "bomberman", name: "Bomberman" },
  codenames: { route: "codenames", name: "Codenames" },
  "the-mind": { route: "themind", name: "The Mind" },
  "the-game": { route: "thegame", name: "The Game" },
  wavelength: { route: "wavelength", name: "Wavelength" },
  "just-one": { route: "justone", name: "Just One" },
  "love-letter": { route: "loveletter", name: "Love Letter" },
  decrypto: { route: "decrypto", name: "Decrypto" },
  hanabi: { route: "hanabi", name: "Hanabi" },
  crawler: { route: "crawler", name: "Totem Crawler" },
  skyjo: { route: "skyjo", name: "Skyjo" },
};

// gameName returns a known game's label, or null for an unrecognised slug (so callers
// never echo an untrusted server string into the UI as if it were a real game).
export function gameName(slug: string): string | null {
  return Object.prototype.hasOwnProperty.call(GAMES, slug) ? GAMES[slug].name : null;
}

// WrongGameNotice renders when this page is joined to a room belonging to another game.
// It offers a link to the correct game (only if the slug is a known one), carrying the
// validated room code so the other page can pre-fill it.
export function WrongGameNotice({ actual, code, onLeave }: { actual: string; code: string; onLeave: () => void }) {
  const info = Object.prototype.hasOwnProperty.call(GAMES, actual) ? GAMES[actual] : undefined;
  return (
    <div className="panel game-notice">
      <h3>Different game</h3>
      {info ? (
        <>
          <p>
            Room <strong>{code}</strong> is a <strong>{info.name}</strong> room, not this game.
          </p>
          <Link className="game-btn" to={`/games/${info.route}?code=${encodeURIComponent(code)}`}>
            Join {info.name}
          </Link>
        </>
      ) : (
        <p>
          Room <strong>{code}</strong> belongs to a different game.
        </p>
      )}
      <button className="game-btn ghost" onClick={onLeave}>
        Back
      </button>
    </div>
  );
}
