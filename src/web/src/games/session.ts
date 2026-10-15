import { useEffect, useState } from "react";

// usePersistedSession keeps a game's "am I in a room" session in sessionStorage so a tab
// reload (a phone dimming or locking, the OS discarding a backgrounded tab) comes straight
// back into the game instead of the join screen. useRoom's resume token (also in
// sessionStorage) then reclaims the held seat within the server's reconnect grace. The
// value survives a reload but not a tab close (sessionStorage), and clears on leave.
//
// Shared by every game page so the reload-resume behaviour lives in ONE place rather than
// being duplicated per game (the transport in useRoom is already shared; this is the thin
// session wrapper around it).
export function usePersistedSession<T>(key: string) {
  const [session, setSession] = useState<T | null>(() => {
    try {
      const s = sessionStorage.getItem(key);
      return s ? (JSON.parse(s) as T) : null;
    } catch {
      return null;
    }
  });
  useEffect(() => {
    try {
      if (session) sessionStorage.setItem(key, JSON.stringify(session));
      else sessionStorage.removeItem(key);
    } catch {
      /* private mode or blocked storage: the game still works, just no reload-resume */
    }
  }, [session, key]);
  return [session, setSession] as const;
}
