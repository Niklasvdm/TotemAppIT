// Mirrors cleanNick and CodeLen in the Go server. The server is the authority
// and refuses a bad nickname at the handshake; these checks only save the user
// a round trip, so keep them in step rather than relying on them.

export const NICK_MAX = 16;
export const CODE_LEN = 4;

const NICK_ALLOWED = /^[\p{L}\p{M}\p{N} '’._-]+$/u;

export function validNick(s: string): boolean {
  const n = s.trim();
  return n.length > 0 && [...n].length <= NICK_MAX && NICK_ALLOWED.test(n);
}
