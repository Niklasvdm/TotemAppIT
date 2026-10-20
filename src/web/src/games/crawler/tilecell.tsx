// One placed map tile: its rotated art, a fog cover, hazard tokens, ground loot (grabbed
// by clicking while your hero is on the tile), monsters and the hero pawns on each square.
import { tileArt, biomeTileDir } from "./logic";
import type { CrTile, CrView, SpriteFor, CrCombat, CrAllyAct, CrSnatch } from "./types";
import { HAZARDS, objInfo, specialLabel, tileKind } from "./data";
import { TotemSprite, MonsterSprite, ObjIcon } from "./sprites";

const ALLY_HINT: Record<string, string> = { revive: "revive this ally", brace: "shield this ally", mend: "heal this ally", carry: "carry this ally (then move with the arrows)" };

export function CrawlerTileCell({ tile, v, focus, debug, spriteFor, highlight, combat, ally, snatch, onAvatar, onPickup, onReopenShop }: { tile: CrTile; v: CrView; focus?: boolean; debug?: boolean; spriteFor: SpriteFor; highlight?: string | null; combat?: CrCombat | null; ally?: CrAllyAct | null; snatch?: CrSnatch | null; onAvatar?: () => void; onPickup?: (square: number) => void; onReopenShop?: () => void }) {
  const heroesHere = v.heroes.filter((h) => h.tile === tile.id); // includes the fallen (death stance)
  const lost = v.ph === "lost"; // a party wipe or collapse: everyone drops to the defeated frame
  const you = v.heroes.find((h) => h.s === v.you);
  // Loot can be grabbed off-turn: any time your hero is alive and standing on this tile
  // (no need to wait for your turn), so a boss's drops stay reachable after the turn moves on.
  const canGrab = !!onPickup && v.ph === "playing" && !!you?.alive && you?.tile === tile.id;
  // Snatch: this tile is one orthogonal step from the hero, so its loot can be pulled in.
  const sdx = snatch ? tile.x - snatch.hx : 0, sdy = snatch ? tile.y - snatch.hy : 0;
  const snatchDir = snatch && Math.abs(sdx) + Math.abs(sdy) === 1 ? (sdy === -1 ? 0 : sdx === 1 ? 1 : sdy === 1 ? 2 : 3) : -1;
  const edgeClass = ["n", "e", "s", "w"].filter((_, d) => tile.edges[d]).map((s) => `open-${s}`).join(" ");
  const art = tileArt(tile.kind, tile.edges, tile.rot, tile.id, biomeTileDir(v.biome)); // id + biome pick the art variant
  return (
    <div
      className={`cr-cell cr-tile ${edgeClass}${tile.isBoss ? " boss" : ""}${tile.weakPoint ? " weakpoint" : ""}${onReopenShop ? " reopen-shop" : ""}`}
      title={tile.weakPoint ? "Hauler's weak-point is in this room — destroy it to drop the boss's immunity" : onReopenShop ? "Click the stall to re-open the kangaroo shop" : undefined}
      data-focus={focus ? "1" : undefined}
      onClick={onReopenShop}
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
          // Every monster on the square is drawn (a square can hold more than one when a
          // swarm or summons overflow); each is offset slightly so none is hidden. A defeated
          // one stays put in its destroyed frame (no stat badge), a living one shows its stats.
          const monsters = tile.monsters.filter((m) => m.square === sq);
          const here = heroesHere.filter((h) => h.square === sq);
          const loot = (tile.loot ?? []).filter((l) => l.square === sq);
          const hazard = (tile.hazards ?? []).find((hz) => hz.square === sq);
          // Enemy stat badge: put it on the side with NO hero pawn so it never prints over the
          // player. Default right; flip left if the right neighbour square holds a hero (or we're
          // on the right column); go above only if both horizontal sides are blocked.
          const heroSquares = new Set(heroesHere.map((h) => h.square));
          const col = sq % 3;
          const badgeRightBlocked = col === 2 || heroSquares.has(sq + 1);
          const badgeLeftBlocked = col === 0 || heroSquares.has(sq - 1);
          const badgeSide = badgeRightBlocked && !badgeLeftBlocked ? "left" : badgeRightBlocked && badgeLeftBlocked ? "above" : "right";
          return (
            <div key={sq} className="cr-sq">
              {hazard && HAZARDS[hazard.kind] && (
                <span className="cr-hazard" title={HAZARDS[hazard.kind].label}>{HAZARDS[hazard.kind].icon}</span>
              )}
              {loot.map((l, i) => {
                const info = objInfo(l.slug);
                // Offset co-located loot so stacked items don't hide each other
                // (same treatment as stacked monsters/hero pawns).
                const lootStyle = i ? { transform: `translate(${i * 9}px, ${i * 9}px)` } : undefined;
                if (canGrab) {
                  return (
                    <button key={`loot-${i}`} type="button" style={lootStyle} className="cr-loot grab" title={`${info.name} on the floor (click to pick up)`} onClick={(e) => { e.stopPropagation(); onPickup!(sq); }}>
                      <ObjIcon slug={l.slug} size={18} />
                    </button>
                  );
                }
                if (snatchDir >= 0) {
                  return (
                    <button key={`loot-${i}`} type="button" style={lootStyle} className="cr-loot snatch" title={`${info.name} — Snatch it from the next tile`} onClick={(e) => { e.stopPropagation(); snatch!.onSnatch(snatchDir); }}>
                      <ObjIcon slug={l.slug} size={18} />
                    </button>
                  );
                }
                return (
                  <span key={`loot-${i}`} style={lootStyle} className="cr-loot" title={`${info.name} on the floor (stand on this tile to pick it up)`}>
                    <ObjIcon slug={l.slug} size={18} />
                  </span>
                );
              })}
              {monsters.map((monster, mi) => {
                const monKey = `${tile.id}-${monster.id}`;
                // Only enemies the active hero can actually reach (own tile / a legally
                // attackable square — the same set the desktop Attack list uses) are
                // tappable; a monster across the map must not respond to taps.
                const reachable = !!combat && monster.alive && combat.reachable.has(monKey); // attackable: glows + ⚔
                return (
                <span
                  key={`mon-${monster.id}`}
                  data-mon={monKey}
                  className={`cr-mon${reachable ? " selectable" : ""}${badgeSide !== "right" ? ` badge-${badgeSide}` : ""}`}
                  style={mi ? { transform: `translate(${mi * 9}px, ${mi * 9}px)` } : undefined}
                >
                  {reachable && (
                    <button
                      className="cr-mon-attack"
                      disabled={!combat!.canHit(monster)}
                      title={combat!.canHit(monster) ? "Attack: spend your lowest die that beats its Armour" : "No unspent die can beat its Armour"}
                      onClick={(e) => { e.stopPropagation(); combat!.onAttack(tile.id, monster); }}
                    >
                      ⚔
                    </button>
                  )}
                  {monster.alive && monster.name && (
                    <span className="cr-boss-name" title={(monster.specials ?? []).map(specialLabel).join(" · ")}>
                      {monster.name}{(monster.specials?.length ?? 0) > 0 ? " ⚡" : ""}
                    </span>
                  )}
                  <MonsterSprite m={monster} highlighted={`${tile.id}-${monster.id}` === highlight} />
                  {monster.alive && (
                    <span className="cr-mon-badge" title={`Armour ${monster.effArmour ?? monster.armour}${(monster.effArmour ?? monster.armour) !== monster.armour ? ` (base ${monster.armour})` : ""} — your die must meet it to hit. Health ${monster.health}/${monster.maxHP}, Damage ${monster.damage}, grants ${monster.reward} Courage on defeat${(monster.specials?.length ?? 0) > 0 ? ". Specials: " + (monster.specials ?? []).map(specialLabel).join(", ") : ""}`}>
                      <span>🛡{monster.effArmour ?? monster.armour}</span>
                      <span>❤{monster.health}</span>
                      <span>💥{monster.damage}</span>
                      <span>✊{monster.reward}</span>
                    </span>
                  )}
                </span>
                );
              })}
              {here.map((h) => {
                const a = spriteFor(h.s);
                const dead = h.downed || !h.alive || lost;
                const isMe = h.s === v.you;
                const avatarClick = isMe && onAvatar; // tapping your own avatar toggles the overlay
                const act = !isMe && ally ? ally.act(h) : null; // tapping an ally does its action
                const carried = ally?.carrying === h.s;
                const clickable = avatarClick || !!act;
                const onPawn = act ? () => ally!.onTap(h) : avatarClick ? () => onAvatar!() : undefined;
                return (
                  <span
                    key={h.s}
                    className={`cr-pawn${h.s === v.turn ? " active" : ""}${isMe ? " you" : ""}${dead ? " dead" : ""}${clickable ? " avatar-btn" : ""}${act ? " ally-tap" : ""}${carried ? " carried" : ""}`}
                    title={act ? `${h.n} — tap to ${ALLY_HINT[act]}` : avatarClick ? h.n + " — tap to show your controls" : h.n + (h.downed ? " (down — revive me)" : "")}
                    role={clickable ? "button" : undefined}
                    onClick={onPawn ? (e) => { e.stopPropagation(); onPawn(); } : undefined}
                  >
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
