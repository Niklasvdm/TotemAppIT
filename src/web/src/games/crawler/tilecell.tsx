// One placed map tile: its rotated art, a fog cover, hazard tokens, ground loot (grabbed
// by clicking while your hero is on the tile), monsters and the hero pawns on each square.
import { tileArt, biomeTileDir } from "./logic";
import type { CrTile, CrView, SpriteFor } from "./types";
import { HAZARDS, objInfo, specialLabel, tileKind } from "./data";
import { TotemSprite, MonsterSprite, ObjIcon } from "./sprites";

export function CrawlerTileCell({ tile, v, focus, debug, spriteFor, onPickup }: { tile: CrTile; v: CrView; focus?: boolean; debug?: boolean; spriteFor: SpriteFor; onPickup?: (square: number) => void }) {
  const heroesHere = v.heroes.filter((h) => h.tile === tile.id); // includes the fallen (death stance)
  const lost = v.ph === "lost"; // a party wipe or collapse: everyone drops to the defeated frame
  const you = v.heroes.find((h) => h.s === v.you);
  // Loot can be grabbed off-turn: any time your hero is alive and standing on this tile
  // (no need to wait for your turn), so a boss's drops stay reachable after the turn moves on.
  const canGrab = !!onPickup && v.ph === "playing" && !!you?.alive && you?.tile === tile.id;
  const edgeClass = ["n", "e", "s", "w"].filter((_, d) => tile.edges[d]).map((s) => `open-${s}`).join(" ");
  const art = tileArt(tile.kind, tile.edges, tile.rot, tile.id); // id picks the art variant
  return (
    <div
      className={`cr-cell cr-tile ${edgeClass}${tile.isBoss ? " boss" : ""}${tile.used ? " used" : ""}`}
      data-focus={focus ? "1" : undefined}
    >
      {art && (
        <div
          className="cr-tile-art"
          style={{ backgroundImage: `url(/sprites/crawler/tiles/${biomeTileDir(v.biome)}/${art.file}.png)`, transform: `rotate(${art.deg}deg)` }}
        />
      )}
      {debug && <span className="cr-tile-kind">{tileKind(tile.kind).label} ({tile.x},{tile.y})</span>}
      {tile.fogged && <div className="cr-fog" title="Sensor fog: move a hero onto or next to this tile to reveal it">🌫️</div>}
      <div className="cr-squares">
        {Array.from({ length: 9 }, (_, sq) => {
          // Each square holds at most one monster; a defeated one stays put in its
          // destroyed frame (no stat badge), a living one shows its stats + Courage reward.
          const monster = tile.monsters.find((m) => m.square === sq);
          const here = heroesHere.filter((h) => h.square === sq);
          const loot = (tile.loot ?? []).filter((l) => l.square === sq);
          const hazard = (tile.hazards ?? []).find((hz) => hz.square === sq);
          return (
            <div key={sq} className="cr-sq">
              {hazard && HAZARDS[hazard.kind] && (
                <span className="cr-hazard" title={HAZARDS[hazard.kind].label}>{HAZARDS[hazard.kind].icon}</span>
              )}
              {loot.map((l, i) => {
                const info = objInfo(l.slug);
                const title = `${info.name} on the floor${canGrab ? " (click to pick up)" : " (stand on this tile to pick it up)"}`;
                return canGrab ? (
                  <button
                    key={`loot-${i}`}
                    type="button"
                    className="cr-loot grab"
                    title={title}
                    onClick={() => onPickup!(sq)}
                  >
                    <ObjIcon slug={l.slug} size={18} />
                  </button>
                ) : (
                  <span key={`loot-${i}`} className="cr-loot" title={title}>
                    <ObjIcon slug={l.slug} size={18} />
                  </span>
                );
              })}
              {monster && (
                <span className="cr-mon">
                  {monster.alive && monster.name && (
                    <span className="cr-boss-name" title={(monster.specials ?? []).map(specialLabel).join(" · ")}>
                      {monster.name}{(monster.specials?.length ?? 0) > 0 ? " ⚡" : ""}
                    </span>
                  )}
                  <MonsterSprite m={monster} />
                  {monster.alive && (
                    <span className="cr-mon-badge" title={`Armour ${monster.effArmour ?? monster.armour}${(monster.effArmour ?? monster.armour) !== monster.armour ? ` (base ${monster.armour})` : ""} — your die must meet it to hit. Health ${monster.health}/${monster.maxHP}, Damage ${monster.damage}, grants ${monster.reward} Courage on defeat${(monster.specials?.length ?? 0) > 0 ? ". Specials: " + (monster.specials ?? []).map(specialLabel).join(", ") : ""}`}>
                      🛡{monster.effArmour ?? monster.armour} ❤{monster.health} 💥{monster.damage} ✊{monster.reward}
                    </span>
                  )}
                </span>
              )}
              {here.map((h) => {
                const a = spriteFor(h.s);
                const dead = !h.alive || lost;
                return (
                  <span key={h.s} className={`cr-pawn${h.s === v.turn ? " active" : ""}${h.s === v.you ? " you" : ""}${dead ? " dead" : ""}`} title={h.n + (h.alive ? "" : " (down)")}>
                    <TotemSprite slug={h.totem} size={40} pose={a.pose} frame={a.frame} flip={a.flip} dead={dead} />
                  </span>
                );
              })}
            </div>
          );
        })}
      </div>
    </div>
  );
}
