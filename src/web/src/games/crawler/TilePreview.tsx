// Tile-art previews: the small line glyph and the rotated-art preview used by the lay
// panel and the map's drop-zone ghost.
import { type Edges, tileArt } from "./logic";

// TileGlyph draws a small square with doorway notches on open edges.
export function TileGlyph({ edges }: { edges: [boolean, boolean, boolean, boolean] }) {
  return (
    <span className="cr-glyph">
      {edges[0] && <i className="cr-door n" />}
      {edges[1] && <i className="cr-door e" />}
      {edges[2] && <i className="cr-door s" />}
      {edges[3] && <i className="cr-door w" />}
    </span>
  );
}

// TilePreview shows a tile's art (rotated to its edges) at an arbitrary size, or the line
// glyph if the kind has no art. Used for the lay panel and the map ghost.
export function TilePreview({ kind, edges, rot = 0, size = 56, dir = "woodland" }: { kind: string; edges: Edges; rot?: number; size?: number; dir?: string }) {
  const art = tileArt(kind, edges, rot);
  if (!art) return <TileGlyph edges={edges} />;
  return (
    <span
      className="cr-tile-prev"
      style={{
        width: size,
        height: size,
        backgroundImage: `url(/sprites/crawler/tiles/${dir}/${art.file}.png)`,
        transform: `rotate(${art.deg}deg)`,
      }}
    />
  );
}
