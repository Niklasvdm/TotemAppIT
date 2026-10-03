package game

import "testing"

func TestNewGridLayout(t *testing.T) {
	g := NewGrid(7)

	for x := 0; x < ArenaW; x++ {
		if !g.Solid(x, 0) || !g.Solid(x, ArenaH-1) {
			t.Fatalf("column %d: arena is not sealed top and bottom", x)
		}
	}
	for y := 0; y < ArenaH; y++ {
		if !g.Solid(0, y) || !g.Solid(ArenaW-1, y) {
			t.Fatalf("row %d: arena is not sealed left and right", y)
		}
	}

	// Interior even/even cells are the pillar lattice; everything else in the
	// interior must be walkable once crates are cleared.
	for y := 1; y < ArenaH-1; y++ {
		for x := 1; x < ArenaW-1; x++ {
			want := x%2 == 0 && y%2 == 0
			if got := g.Solid(x, y); got != want {
				t.Fatalf("solid(%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

func TestSpawnsAreOpenWithAnEscape(t *testing.T) {
	// Several seeds, because the crate layer is the random part.
	for seed := uint64(0); seed < 50; seed++ {
		g := NewGrid(seed)
		for slot := 0; slot < MaxPlayers; slot++ {
			sx, sy := g.Spawn(slot)
			if g.Blocked(sx, sy) {
				t.Fatalf("seed %d slot %d: spawn (%d,%d) is blocked", seed, slot, sx, sy)
			}
			// At least two ways out, or the player is walled in at the start.
			exits := 0
			for _, d := range blastDirs {
				if !g.Blocked(sx+d[0], sy+d[1]) {
					exits++
				}
			}
			if exits < 2 {
				t.Fatalf("seed %d slot %d: spawn (%d,%d) has %d exits, want >= 2",
					seed, slot, sx, sy, exits)
			}
		}
	}
}

// A player's first bomb is dropped on their own spawn. If every tile they can
// reach is inside that blast, the opening move is unavoidably fatal — so every
// spawn must have a standable tile outside it.
func TestSpawnHasARetreatFromTheOpeningBomb(t *testing.T) {
	for seed := uint64(0); seed < 200; seed++ {
		g := NewGrid(seed)
		for slot := 0; slot < MaxPlayers; slot++ {
			sx, sy := g.Spawn(slot)

			blast := map[[2]int]bool{{sx, sy}: true}
			for _, d := range blastDirs {
				for i := 1; i <= basePower; i++ {
					x, y := sx+d[0]*i, sy+d[1]*i
					if g.Solid(x, y) {
						break
					}
					blast[[2]int{x, y}] = true
					if g.Crate(x, y) {
						break
					}
				}
			}

			safe := 0
			for _, d := range blastDirs {
				for i := 1; i <= spawnClearDepth; i++ {
					tile := [2]int{sx + d[0]*i, sy + d[1]*i}
					if g.Blocked(tile[0], tile[1]) {
						break // can't walk past a wall or crate to reach it
					}
					if !blast[tile] {
						safe++
					}
				}
			}
			if safe == 0 {
				t.Fatalf("seed %d slot %d: spawn (%d,%d) has no tile outside its own opening blast",
					seed, slot, sx, sy)
			}
		}
	}
}

func TestGridIsSeedDeterministic(t *testing.T) {
	a, b, c := NewGrid(99), NewGrid(99), NewGrid(100)
	if a.CrateString() != b.CrateString() {
		t.Fatal("same seed produced different crate layers")
	}
	if a.CrateString() == c.CrateString() {
		t.Fatal("different seeds produced identical crate layers")
	}
}

func TestEncodedLayersMatchArenaSize(t *testing.T) {
	g := NewGrid(3)
	if n := len(g.SolidString()); n != ArenaW*ArenaH {
		t.Fatalf("SolidString length = %d, want %d", n, ArenaW*ArenaH)
	}
	if n := len(g.CrateString()); n != ArenaW*ArenaH {
		t.Fatalf("CrateString length = %d, want %d", n, ArenaW*ArenaH)
	}
}

func TestBreakCrate(t *testing.T) {
	g := NewGrid(1)
	// Find a standing crate; the layer is dense, so one always exists.
	var cx, cy int
	for y := 1; y < ArenaH-1 && cx == 0; y++ {
		for x := 1; x < ArenaW-1; x++ {
			if g.Crate(x, y) {
				cx, cy = x, y
				break
			}
		}
	}
	if cx == 0 {
		t.Fatal("no crate found in a freshly generated grid")
	}

	if !g.BreakCrate(cx, cy) {
		t.Fatal("BreakCrate reported nothing to break")
	}
	if g.Crate(cx, cy) {
		t.Fatal("crate still standing after BreakCrate")
	}
	if g.BreakCrate(cx, cy) {
		t.Fatal("BreakCrate broke the same crate twice")
	}
	if g.Blocked(cx, cy) {
		t.Fatal("cleared crate tile is still blocked")
	}
}
