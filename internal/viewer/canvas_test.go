package viewer

import (
	"math"
	"slices"
	"testing"
	"time"

	"github.com/Castux/wgen/internal/config"
)

var (
	testSea  = config.Color{66, 66, 125}
	testLand = config.Color{101, 72, 31}
)

func seaCanvas(width, height int) *canvas {
	pixels := make([]config.Color, width*height)
	for i := range pixels {
		pixels[i] = testSea
	}
	return newCanvas(width, height, pixels)
}

func (c *canvas) count(color config.Color) int {
	n := 0
	for _, p := range c.pixels {
		if p == color {
			n++
		}
	}
	return n
}

func TestStamp(t *testing.T) {
	var shapes [][]config.Color
	for seed := range uint64(3) {
		c := seaCanvas(200, 200)
		c.stamp(100, 100, 30, testLand, seed, brushNatural)

		// About a disk of that radius, not a disk
		area := float64(c.count(testLand))
		if disk := math.Pi * 30 * 30; area < 0.4*disk || area > 2.5*disk {
			t.Errorf("seed %d: area %v, disk %v", seed, area, disk)
		}
		if c.at(100, 100) != testLand || c.at(0, 0) != testSea {
			t.Errorf("seed %d: center or corner wrong", seed)
		}
		shapes = append(shapes, c.pixels)
	}
	if slices.Equal(shapes[0], shapes[1]) || slices.Equal(shapes[1], shapes[2]) {
		t.Error("stamps of different seeds are the same")
	}

	// Clipped at the edges
	c := seaCanvas(50, 50)
	if r := c.stamp(0, 0, 40, testLand, 1, brushNatural); r.Min.X < 0 || r.Max.X > 50 {
		t.Errorf("rectangle %v", r)
	}
}

func TestStrokeUndo(t *testing.T) {
	c := seaCanvas(300, 100)
	original := slices.Clone(c.pixels)

	seed := uint64(0)
	next := func() uint64 { seed++; return seed }
	c.beginStroke(20, 50, 10, testLand, next(), brushNatural)
	c.continueStroke(280, 50, 10, testLand, next)
	if !c.endStroke() {
		t.Fatal("stroke changed nothing")
	}
	painted := slices.Clone(c.pixels)

	// The stroke goes all the way
	if c.at(150, 50) != testLand || c.at(270, 50) != testLand {
		t.Error("stroke not continuous")
	}

	if r := c.undoEdit(); r.Empty() || !slices.Equal(c.pixels, original) {
		t.Error("undo doesn't restore the map")
	}
	if r := c.redoEdit(); r.Empty() || !slices.Equal(c.pixels, painted) {
		t.Error("redo doesn't repaint")
	}
	c.undoEdit()

	// A new stroke clears the redo history
	c.beginStroke(50, 50, 5, testLand, next(), brushNatural)
	c.endStroke()
	if r := c.redoEdit(); !r.Empty() {
		t.Error("redo after a new stroke")
	}

	// A stroke changing nothing isn't an edit
	undos := len(c.undo)
	c.beginStroke(50, 50, 5, testLand, 99, brushNatural)
	c.beginStroke(50, 50, 5, testLand, 99, brushNatural)
	if c.endStroke(); len(c.undo) != undos {
		t.Error("empty stroke recorded")
	}
}

func TestCanvasRGBA(t *testing.T) {
	c := seaCanvas(4, 3)
	c.pixels[c.index(1, 0)] = testLand // top row
	data := c.rgba(c.bounds())
	if len(data) != 4*12 || data[4] != testLand[0] || data[0] != testSea[0] {
		t.Errorf("rgba %v", data[:8])
	}
	// Bottom row first in pixels: the top row is the last
	if c.pixels[2*4+1] != testLand {
		t.Error("rows not bottom first")
	}
}

// Protected pixels are left alone: the shoreline lock.
func TestStampProtect(t *testing.T) {
	c := seaCanvas(100, 100)
	hills := config.Color{209, 184, 134}

	// Land on the left half
	for row := range 100 {
		for x := range 50 {
			c.pixels[c.index(x, row)] = testLand
		}
	}
	before := c.count(testSea)

	// Land brush, water protected: only land changes
	c.protect = func(p config.Color) bool { return p == testSea }
	c.stamp(50, 50, 30, hills, 1, brushNatural)
	if c.count(testSea) != before || c.count(hills) == 0 {
		t.Errorf("land brush: sea %d, was %d; hills %d", c.count(testSea), before, c.count(hills))
	}
	for row := range 100 {
		for x := 50; x < 100; x++ {
			if c.at(x, row) != testSea {
				t.Fatalf("sea painted at %d, %d", x, row)
			}
		}
	}

	// Unprotected, the stamp crosses the shore
	c.protect = nil
	c.stamp(50, 50, 30, hills, 1, brushNatural)
	if c.count(testSea) >= before {
		t.Error("unprotected stamp left the sea alone")
	}
}

func TestHardBrushes(t *testing.T) {
	c := seaCanvas(100, 100)
	c.stamp(50, 50, 10, testLand, 1, brushSquare)
	if n := c.count(testLand); n != 400 {
		t.Errorf("square of radius 10: %d pixels", n)
	}

	c = seaCanvas(100, 100)
	c.stamp(50, 50, 10, testLand, 1, brushRound)
	if n := float64(c.count(testLand)); math.Abs(n-math.Pi*100) > 20 {
		t.Errorf("disk of radius 10: %v pixels", n)
	}
	if c.at(50, 50) != testLand || c.at(50, 39) != testSea || c.at(59, 59) != testSea {
		t.Error("disk shape")
	}

	// Strokes of hard brushes are continuous too
	c = seaCanvas(200, 50)
	seed := uint64(0)
	c.beginStroke(10, 25, 5, testLand, 1, brushSquare)
	c.continueStroke(190, 25, 5, testLand, func() uint64 { seed++; return seed })
	c.endStroke()
	for x := 10; x < 190; x++ {
		if c.at(x, 25) != testLand {
			t.Fatalf("gap at %d", x)
		}
	}
}

func TestRecolorIsland(t *testing.T) {
	c := seaCanvas(200, 200)
	hills := config.Color{209, 184, 134}
	c.island([]config.Color{testLand, hills}, 3)
	if c.count(testLand) == 0 || c.count(hills) == 0 || c.at(0, 0) != testSea {
		t.Fatalf("island: land %d, hills %d", c.count(testLand), c.count(hills))
	}

	c.beginStroke(10, 10, 5, testLand, 1, brushRound)
	c.endStroke()
	other := config.Color{1, 2, 3}
	c.recolor(testLand, other)
	if c.count(testLand) != 0 || c.count(other) == 0 {
		t.Error("recolor")
	}
	c.undoEdit()
	if c.count(testLand) != 0 {
		t.Error("undo brought back the old color")
	}
}

// Big stamps are fast enough for a new map's island.
func TestIslandSpeed(t *testing.T) {
	c := seaCanvas(2048, 2048)
	start := time.Now()
	c.island([]config.Color{testLand, {1, 2, 3}, {4, 5, 6}}, 1)
	if took := time.Since(start); took > 2*time.Second {
		t.Errorf("island of a 2048 px map took %v", took)
	}
}
