import { describe, it, expect } from "vitest";
import { renderToStaticMarkup } from "react-dom/server";
import { LocalSeats, type LocalPlayer } from "./LocalSeats";

// Structural tests for the local-multiplayer tab wrapper: the initial render (active seat
// 0) is enough to check the tab bar, the head crown, the "+ add" affordance and that each
// seat is rendered with its index as seatKey.
const seat = (p: LocalPlayer, key: string, active: boolean) =>
  <span data-key={key} data-active={String(active)}>{p.name}</span>;

describe("LocalSeats", () => {
  it("renders a tab per player, crowns the head, and shows + add when a menu is given", () => {
    const players = [{ name: "Niklas" }, { name: "Luca" }];
    const html = renderToStaticMarkup(
      <LocalSeats players={players} renderSeat={seat} onAddPlayer={() => {}} addMenu={() => <i />} />,
    );
    expect(html).toContain("Niklas");
    expect(html).toContain("Luca");
    expect(html).toContain("👑"); // head player (index 0)
    expect(html).toContain("+ add");
    // each seat rendered with its index as the seatKey
    expect(html).toContain('data-key="0"');
    expect(html).toContain('data-key="1"');
  });

  it("shows no tab bar for a lone player with no add menu", () => {
    const html = renderToStaticMarkup(
      <LocalSeats players={[{ name: "Solo" }]} renderSeat={seat} />,
    );
    expect(html).not.toContain("local-tabs");
    expect(html).toContain("Solo");
  });

  it("offers + add for a lone player when a menu is given", () => {
    const html = renderToStaticMarkup(
      <LocalSeats players={[{ name: "Solo" }]} renderSeat={seat} onAddPlayer={() => {}} addMenu={() => <i />} />,
    );
    expect(html).toContain("local-tabs");
    expect(html).toContain("+ add");
  });

  it("hides every seat but the active one", () => {
    const players = [{ name: "A" }, { name: "B" }];
    const html = renderToStaticMarkup(<LocalSeats players={players} renderSeat={seat} />);
    // seat 0 is active (not hidden), seat 1 is hidden
    expect(html).toContain('data-active="true"');
    expect(html).toContain('data-active="false"');
    expect(html).toContain("hidden");
  });
});
