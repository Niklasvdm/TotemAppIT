// The scrollable map grid: placed tiles (CrawlerTileCell), frontier markers, and — while
// laying — the glowing drop zones with a ghost of the tile at the chosen rotation.
import { useEffect, useRef } from "react";
import { tileArt, biomeTileDir } from "./logic";
import type { CrView, LayState, SpriteFor } from "./types";
import { TileGlyph } from "./TilePreview";
import { CrawlerTileCell } from "./tilecell";

// CrawlerMap keeps the `focus` cell centred (your tile, or the tile being laid); `lay`
// turns every legal frontier cell into a drop-zone with a tile ghost.
export function CrawlerMap({ v, focus, lay, debug, spriteFor, onPickup }: { v: CrView; focus: { x: number; y: number } | null; lay: LayState | null; debug?: boolean; spriteFor: SpriteFor; onPickup?: (square: number) => void }) {
  const scrollRef = useRef<HTMLDivElement>(null);
  // Keep the focus cell centred in the scroll box as the map grows / you move.
  useEffect(() => {
    const c = scrollRef.current;
    if (!c) return;
    const el = c.querySelector('[data-focus="1"]') as HTMLElement | null;
    if (!el) return;
    const cr = c.getBoundingClientRect();
    const er = el.getBoundingClientRect();
    c.scrollLeft += er.left + er.width / 2 - (cr.left + cr.width / 2);
    c.scrollTop += er.top + er.height / 2 - (cr.top + cr.height / 2);
  }, [focus?.x, focus?.y, v.tiles.length]);

  const coords = [...v.tiles.map((t) => [t.x, t.y]), ...v.frontiers.map((f) => [f.x, f.y])];
  if (lay) lay.options.forEach((o) => coords.push([o.x, o.y]));
  if (coords.length === 0) return null;
  const xs = coords.map((c) => c[0]);
  const ys = coords.map((c) => c[1]);
  const minX = Math.min(...xs), maxX = Math.max(...xs);
  const minY = Math.min(...ys), maxY = Math.max(...ys);
  const cols = maxX - minX + 1;
  const tileAt = (x: number, y: number) => v.tiles.find((t) => t.x === x && t.y === y);
  const frontierAt = (x: number, y: number) => v.frontiers.some((f) => f.x === x && f.y === y);
  const layAt = (x: number, y: number) => (lay ? lay.options.filter((o) => o.x === x && o.y === y) : []);
  const isFocus = (x: number, y: number) => !!focus && focus.x === x && focus.y === y;

  const cells = [];
  for (let y = minY; y <= maxY; y++) {
    for (let x = minX; x <= maxX; x++) {
      const key = `${x},${y}`;
      const tile = tileAt(x, y);
      const atCell = layAt(x, y);
      if (lay && atCell.length > 0) {
        // Show the chosen rotation where it fits; otherwise this cell's first legal one.
        const hasRot = atCell.some((o) => o.rotation === lay.rot);
        const ghost = atCell.find((o) => o.rotation === lay.rot) ?? atCell[0];
        const art = tileArt(lay.kind, ghost.edges, ghost.rotation);
        cells.push(
          <div
            key={key}
            className={`cr-cell cr-lay-drop${hasRot ? " sel" : " alt"}${isFocus(x, y) ? " focus" : ""}`}
            data-focus={isFocus(x, y) ? "1" : undefined}
            onDragOver={(e) => e.preventDefault()}
            onDrop={(e) => { e.preventDefault(); lay.place(x, y); }}
            onClick={() => lay.place(x, y)}
            title={hasRot ? "Drop or click to place here (your rotation)" : "Drop or click to place here (this cell's rotation)"}
          >
            {art ? (
              <div className="cr-tile-art" style={{ backgroundImage: `url(/sprites/crawler/tiles/${biomeTileDir(v.biome)}/${art.file}.png)`, transform: `rotate(${art.deg}deg)` }} />
            ) : (
              <TileGlyph edges={ghost.edges} />
            )}
          </div>,
        );
      } else if (tile) {
        cells.push(<CrawlerTileCell key={key} tile={tile} v={v} focus={isFocus(x, y)} debug={debug} spriteFor={spriteFor} onPickup={onPickup} />);
      } else if (frontierAt(x, y)) {
        cells.push(<div key={key} className="cr-cell cr-frontier">?</div>);
      } else {
        cells.push(<div key={key} className="cr-cell cr-empty" />);
      }
    }
  }
  return (
    <div className="cr-map-scroll" ref={scrollRef}>
      <div className="cr-map" style={{ gridTemplateColumns: `repeat(${cols}, var(--cr-cell))` }}>
        {cells}
      </div>
    </div>
  );
}
