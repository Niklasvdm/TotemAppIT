// The scrollable map grid: placed tiles (CrawlerTileCell), frontier markers, and — while
// laying — the glowing drop zones with a ghost of the tile at the chosen rotation.
import { useEffect, useRef, type PointerEvent as ReactPointerEvent, type MouseEvent as ReactMouseEvent } from "react";
import { tileArt, biomeTileDir } from "./logic";
import type { CrView, LayState, SpriteFor, CrCombat, CrAllyAct, CrSnatch } from "./types";
import { TileGlyph } from "./TilePreview";
import { CrawlerTileCell } from "./tilecell";

// CrawlerMap keeps the `focus` cell centred (your tile, or the tile being laid); `lay`
// turns every legal frontier cell into a drop-zone with a tile ghost.
export function CrawlerMap({ v, focus, lay, debug, spriteFor, highlight, combat, ally, snatch, onAvatar, onPickup, onReopenShop }: { v: CrView; focus: { x: number; y: number } | null; lay: LayState | null; debug?: boolean; spriteFor: SpriteFor; highlight?: string | null; combat?: CrCombat | null; ally?: CrAllyAct | null; snatch?: CrSnatch | null; onAvatar?: () => void; onPickup?: (square: number) => void; onReopenShop?: (tileId: number) => void }) {
  const scrollRef = useRef<HTMLDivElement>(null);
  // Keep the focus cell IN VIEW as the map grows / you move, but do not recentre on every
  // step: only nudge (smoothly) when it drifts outside a comfortable inner box. Hard-centring
  // on each move made the whole map jump around, and it fought manual panning.
  useEffect(() => {
    const c = scrollRef.current;
    if (!c) return;
    const el = c.querySelector('[data-focus="1"]') as HTMLElement | null;
    if (!el) return;
    const cr = c.getBoundingClientRect();
    const er = el.getBoundingClientRect();
    const mx = cr.width * 0.28, my = cr.height * 0.28; // keep it within the inner ~44% box
    let dx = 0, dy = 0;
    if (er.left < cr.left + mx) dx = er.left - (cr.left + mx);
    else if (er.right > cr.right - mx) dx = er.right - (cr.right - mx);
    if (er.top < cr.top + my) dy = er.top - (cr.top + my);
    else if (er.bottom > cr.bottom - my) dy = er.bottom - (cr.bottom - my);
    if (dx || dy) c.scrollBy({ left: dx, top: dy, behavior: "smooth" });
  }, [focus?.x, focus?.y, v.tiles.length]);

  // Drag-to-pan with the mouse (touch already pans natively). A small movement threshold tells
  // a pan from a click, and a real drag suppresses the click that would otherwise lay a tile
  // or select a monster. Pointer capture keeps the drag alive if the cursor leaves the box.
  const drag = useRef<{ x: number; y: number; sl: number; st: number; moved: boolean } | null>(null);
  const dragged = useRef(false);
  const onPointerDown = (e: ReactPointerEvent) => {
    if (e.pointerType !== "mouse" || e.button !== 0) return;
    const c = scrollRef.current;
    if (!c) return;
    drag.current = { x: e.clientX, y: e.clientY, sl: c.scrollLeft, st: c.scrollTop, moved: false };
    dragged.current = false;
  };
  const onPointerMove = (e: ReactPointerEvent) => {
    const d = drag.current, c = scrollRef.current;
    if (!d || !c) return;
    const dx = e.clientX - d.x, dy = e.clientY - d.y;
    if (!d.moved && Math.hypot(dx, dy) > 6) { d.moved = true; c.setPointerCapture(e.pointerId); c.classList.add("dragging"); }
    if (d.moved) { c.scrollLeft = d.sl - dx; c.scrollTop = d.st - dy; }
  };
  const endDrag = (e: ReactPointerEvent) => {
    const d = drag.current, c = scrollRef.current;
    if (d?.moved) dragged.current = true; // a real drag just happened: swallow the trailing click
    if (c?.hasPointerCapture?.(e.pointerId)) c.releasePointerCapture(e.pointerId);
    c?.classList.remove("dragging");
    drag.current = null;
  };
  const onClickCapture = (e: ReactMouseEvent) => {
    if (dragged.current) { e.stopPropagation(); e.preventDefault(); dragged.current = false; }
  };

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
  // Show the per-cell rotate button only when the drawn tile actually has more than one legal
  // orientation to cycle through (a cross or a fully-constrained spot has just one).
  const canRotate = lay ? new Set(lay.options.map((o) => o.rotation)).size > 1 : false;
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
        const art = tileArt(lay.kind, ghost.edges, ghost.rotation, 0, biomeTileDir(v.biome));
        cells.push(
          <div
            key={key}
            className={`cr-cell cr-lay-drop${hasRot ? " sel" : " alt"}${isFocus(x, y) ? " focus" : ""}`}
            data-focus={isFocus(x, y) ? "1" : undefined}
            onDragOver={(e) => e.preventDefault()}
            onDrop={(e) => { e.preventDefault(); lay.place(x, y); }}
            onClick={() => lay.place(x, y)}
            onWheel={(e) => { e.preventDefault(); lay.rotate(e.deltaY > 0 ? 1 : -1); }}
            title={hasRot ? "Click the centre ⟳ to rotate · click anywhere else (or drop) to lay it here" : "Click the centre ⟳ to rotate · click anywhere else (or drop) to lay it here (this cell's rotation)"}
          >
            {art ? (
              <div className="cr-tile-art" style={{ backgroundImage: `url(/sprites/crawler/tiles/${biomeTileDir(v.biome)}/${art.file}.png)`, transform: `rotate(${art.deg}deg)` }} />
            ) : (
              <TileGlyph edges={ghost.edges} />
            )}
            {canRotate && (
              <button
                className="cr-lay-rotate"
                title="Rotate this tile (or scroll / r)"
                onClick={(e) => { e.stopPropagation(); lay.rotate(1); }}
              >
                ⟳
              </button>
            )}
          </div>,
        );
      } else if (tile) {
        cells.push(<CrawlerTileCell key={key} tile={tile} v={v} focus={isFocus(x, y)} debug={debug} spriteFor={spriteFor} highlight={highlight} combat={combat} ally={ally} snatch={snatch} onAvatar={onAvatar} onPickup={onPickup} onReopenShop={onReopenShop && tile.kind === "shop" ? () => onReopenShop(tile.id) : undefined} />);
      } else if (frontierAt(x, y)) {
        cells.push(<div key={key} className="cr-cell cr-frontier">?</div>);
      } else {
        cells.push(<div key={key} className="cr-cell cr-empty" />);
      }
    }
  }
  return (
    <div
      className="cr-map-scroll"
      ref={scrollRef}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={endDrag}
      onPointerCancel={endDrag}
      onClickCapture={onClickCapture}
    >
      <div className="cr-map" style={{ gridTemplateColumns: `repeat(${cols}, var(--cr-cell))` }}>
        {cells}
      </div>
    </div>
  );
}
