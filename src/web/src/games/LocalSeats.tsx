import { type ReactNode, useEffect, useRef, useState } from "react";

// LocalSeats is the shared "local multiplayer" wrapper for the turn-based games: several
// players share one device, each joining the same room as its own seat (its own socket,
// via useRoom's seatKey). It renders a tab per local player (the head player, index 0,
// who owns the device, is crowned), keeps every seat mounted so their private views stay
// live, and shows only the active one. The "+ add" tab opens the game's own selection
// menu (name, totem, ...) and the new player joins live.
//
// The engine is already multi-seat, so this needs no server change: a local player is
// just another seat in the room, and per-seat private views (a Codenames spymaster, a
// Hanabi hand) are exactly why each local player needs their own tab rather than a shared
// board. `renderSeat` returns the game's room component for one player; `addMenu`, when
// given, is that game's selection menu for adding another local player.

export interface LocalPlayer {
  name: string;
  meta?: string; // per-game selection (totem slug, ...), passed to the room as useRoom meta
}

export function LocalSeats({
  players,
  onAddPlayer,
  renderSeat,
  addMenu,
  tabColor,
  focusSeatKey,
  handoff = false,
  maxPlayers = 6,
}: {
  players: LocalPlayer[];
  onAddPlayer?: (p: LocalPlayer) => void;
  renderSeat: (player: LocalPlayer, seatKey: string, active: boolean) => ReactNode;
  addMenu?: (submit: (p: LocalPlayer) => void, cancel: () => void) => ReactNode;
  // Optional per-tab accent (e.g. a Codenames team colour). Returns a CSS colour for the
  // player's tab, or undefined for the default styling.
  tabColor?: (player: LocalPlayer, seatKey: string) => string | undefined;
  // When a seat reports it is now its turn, the game sets focusSeatKey to that seat so the
  // device switches to the player whose turn it is (auto-switch on end of turn).
  focusSeatKey?: string;
  // Hidden-information games (Codenames, Hanabi) set handoff so switching players shows a
  // "pass the device" cover first, hiding the incoming player's private view until they
  // tap Ready. Open-info co-op (the Crawler) leaves it off.
  handoff?: boolean;
  maxPlayers?: number;
}) {
  const [active, setActive] = useState(0);
  const [adding, setAdding] = useState(false);
  const [covered, setCovered] = useState(false);
  const act = Math.max(0, Math.min(active, players.length - 1));
  const canAdd = !!addMenu && !!onAddPlayer && players.length < maxPlayers;
  const showTabs = players.length > 1 || canAdd;

  // switchTo changes the visible seat; with handoff on, switching to a DIFFERENT player
  // first drops a "pass the device" cover so the outgoing player never sees the incoming
  // one's private view.
  const switchTo = (i: number) => {
    if (i !== act && handoff && players.length > 1) setCovered(true);
    setActive(i);
    setAdding(false);
  };

  // Auto-switch to whichever seat's turn it is (the game drives focusSeatKey).
  const lastFocus = useRef<string | undefined>(undefined);
  useEffect(() => {
    if (focusSeatKey == null || focusSeatKey === lastFocus.current) return;
    lastFocus.current = focusSeatKey;
    const i = Number(focusSeatKey);
    if (!Number.isNaN(i) && i >= 0 && i < players.length) switchTo(i);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [focusSeatKey]);

  return (
    <div className="local-seats">
      {showTabs && (
        <div className="local-tabs" role="tablist" aria-label="Local players">
          {players.map((p, i) => {
            const color = tabColor?.(p, String(i));
            return (
              <button
                key={i}
                role="tab"
                aria-selected={i === act}
                className={`local-tab${i === act ? " active" : ""}${color ? " teamed" : ""}`}
                style={color ? { background: color, borderColor: color, color: "#fff" } : undefined}
                title={i === 0 ? "Head player (this device)" : "Local player"}
                onClick={() => switchTo(i)}
              >
                {i === 0 ? "👑 " : ""}
                {p.name || `Player ${i + 1}`}
              </button>
            );
          })}
          {canAdd && (
            <button className="local-tab add" onClick={() => setAdding(true)} title="Add a player on this device">
              + add
            </button>
          )}
        </div>
      )}

      {adding && addMenu && onAddPlayer && (
        <div className="local-add panel">
          {addMenu(
            (p) => {
              onAddPlayer(p);
              switchTo(players.length); // focus the newly added seat (its index is the old length)
            },
            () => setAdding(false),
          )}
        </div>
      )}

      {covered && (
        // Pass-the-device cover for hidden-information games: hides the board until the
        // incoming player confirms they are holding the device.
        <div className="local-handoff panel">
          <div className="local-handoff-name">
            {act === 0 ? "👑 " : ""}
            {players[act]?.name || `Player ${act + 1}`}
          </div>
          <p className="muted-note">Pass the device to this player, then tap when you are holding it.</p>
          <button className="game-btn" onClick={() => setCovered(false)}>I'm {players[act]?.name || "ready"} — show my view</button>
        </div>
      )}

      {players.map((p, i) => (
        // Every seat stays mounted (its socket and private view stay warm); only the
        // active one is shown, and the pass cover hides it until the player is ready.
        <div key={i} className="local-seat" hidden={i !== act || covered}>
          {renderSeat(p, String(i), i === act && !covered)}
        </div>
      ))}
    </div>
  );
}

// NameAddForm is a minimal selection menu (just a name) for games whose only per-player
// choice is a nickname; a game with more (the Crawler's totem) supplies its own addMenu.
export function NameAddForm({
  submit,
  cancel,
  maxLen = 20,
}: {
  submit: (p: LocalPlayer) => void;
  cancel: () => void;
  maxLen?: number;
}) {
  const [name, setName] = useState("");
  const go = () => {
    const n = name.trim();
    if (n) submit({ name: n });
  };
  return (
    <div className="local-add-form">
      <input
        className="trait-search"
        autoFocus
        value={name}
        maxLength={maxLen}
        placeholder="Player name"
        onChange={(e) => setName(e.target.value)}
        onKeyDown={(e) => e.key === "Enter" && go()}
      />
      <button className="game-btn" onClick={go}>Add</button>
      <button className="game-btn ghost" onClick={cancel}>Cancel</button>
    </div>
  );
}
