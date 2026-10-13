// The kangaroo's stall, shown as a callout over the board while you stand on a shop
// tile's centre: each item shows its icon, name, cost and effect; Buy spends Courage.
import type { CrHero } from "./types";
import { objInfo, OBJECT_CAP } from "./data";
import { ObjIcon } from "./sprites";

export function CrawlerShopCallout({ me, shop, onSend }: {
  me: CrHero;
  shop: { slug: string; name: string; cost: number }[];
  onSend: (msg: object) => void;
}) {
  const packFull = me.objects.length >= (me.slots || OBJECT_CAP);
  return (
    <div className="cr-shop-callout">
      <div className="cr-shop-callout-head">
        🛒 Kangaroo shop <span className="muted-note">({me.courage}✊ · {me.objects.length}/{me.slots || OBJECT_CAP} slots)</span>
      </div>
      {packFull && <div className="cr-shop-note">Pack full: use or drop something first.</div>}
      {shop.length === 0 && <div className="cr-shop-note">Sold out this level.</div>}
      <ul className="cr-shop-list">
        {shop.map((s) => {
          const info = objInfo(s.slug);
          const afford = me.courage >= s.cost && !packFull;
          return (
            <li key={s.slug} className="cr-shop-item">
              <ObjIcon slug={s.slug} size={26} />
              <div className="cr-shop-item-text">
                <strong>{info.name}</strong>
                <span className="muted-note">{info.effect}</span>
              </div>
              <button className="game-btn" disabled={!afford} onClick={() => onSend({ t: "buy", obj: s.slug })}>
                {s.cost}✊
              </button>
            </li>
          );
        })}
      </ul>
    </div>
  );
}
