// The kangaroo's stall, shown as a callout over the board while you stand on a shop tile's
// centre. Two tabs: BUY (the stall's stock, spends Courage; permanents show their rarity) and
// SELL (your pack, +1 Courage each; blocked at the Courage cap since it would be wasted).
import { useState } from "react";
import type { CrHero } from "./types";
import { objInfo, OBJECT_CAP, COURAGE_CAP, RARITY_LABEL } from "./data";
import { ObjIcon } from "./sprites";

export function CrawlerShopCallout({ me, shop, onSend, onClose }: {
  me: CrHero;
  shop: { slug: string; name: string; cost: number }[];
  onSend: (msg: object) => void;
  onClose?: () => void;
}) {
  const [tab, setTab] = useState<"buy" | "sell">("buy");
  const packFull = me.objects.length >= (me.slots || OBJECT_CAP);
  const atCap = me.courage >= COURAGE_CAP;
  return (
    <div className="cr-shop-callout">
      <div className="cr-shop-callout-head">
        <span>🛒 Kangaroo shop <span className="muted-note">({me.courage}✊ · {me.objects.length}/{me.slots || OBJECT_CAP} slots)</span></span>
        {onClose && (
          <button type="button" className="cr-shop-close" title="Close the shop (click the stall to re-open)" onClick={onClose}>✕</button>
        )}
      </div>
      <div className="cr-shop-tabs">
        <button type="button" className={`cr-shop-tab${tab === "buy" ? " on" : ""}`} onClick={() => setTab("buy")}>Buy</button>
        <button type="button" className={`cr-shop-tab${tab === "sell" ? " on" : ""}`} onClick={() => setTab("sell")}>Sell</button>
      </div>

      {tab === "buy" ? (
        <>
          {packFull && <div className="cr-shop-note">Pack full: use, drop or sell something first.</div>}
          {shop.length === 0 && <div className="cr-shop-note">Sold out this level.</div>}
          <ul className="cr-shop-list">
            {shop.map((s) => {
              const info = objInfo(s.slug);
              const afford = me.courage >= s.cost && !packFull;
              return (
                <li key={s.slug} className="cr-shop-item">
                  <ObjIcon slug={s.slug} size={26} />
                  <div className="cr-shop-item-text">
                    <strong>{info.name}{info.perm && <span className={`cr-rarity cr-rarity-${info.rarity}`}>{RARITY_LABEL[info.rarity]}</span>}</strong>
                    <span className="muted-note">{info.effect}</span>
                  </div>
                  <button className="game-btn" disabled={!afford} onClick={() => onSend({ t: "buy", obj: s.slug })}>
                    {s.cost}✊
                  </button>
                </li>
              );
            })}
          </ul>
        </>
      ) : (
        <>
          {me.objects.length === 0 && <div className="cr-shop-note">Nothing to sell.</div>}
          {atCap && me.objects.length > 0 && <div className="cr-shop-note">Courage is maxed ({COURAGE_CAP}): selling would be wasted.</div>}
          <ul className="cr-shop-list">
            {me.objects.map((slug, i) => {
              const info = objInfo(slug);
              return (
                <li key={`${slug}-${i}`} className="cr-shop-item">
                  <ObjIcon slug={slug} size={26} />
                  <div className="cr-shop-item-text">
                    <strong>{info.name}{info.perm && <span className={`cr-rarity cr-rarity-${info.rarity}`}>{RARITY_LABEL[info.rarity]}</span>}</strong>
                    <span className="muted-note">{info.effect}</span>
                  </div>
                  <button className="game-btn" disabled={atCap} title={atCap ? "Courage is already maxed" : "Sell for 1 Courage"} onClick={() => onSend({ t: "sell", obj: slug })}>
                    +1✊
                  </button>
                </li>
              );
            })}
          </ul>
        </>
      )}
    </div>
  );
}
