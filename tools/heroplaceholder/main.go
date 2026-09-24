// Command heroplaceholder writes a PLACEHOLDER hero for the PNG hero path
// (M5.3): data/strigoi/hero/placeholder/, six sprite sheets (idle, walk,
// attack, hit, death, dead) in eight directions, each with its .png.json
// sheet manifest, and the hero.json that names them.
//
// It is a stick-simple figure -- a long coat, a tall white cap folded back,
// a blade -- drawn so the pipeline can be proved end to end before the real
// Janissary exists. The real one is Josh's and GPT's to make; it drops in
// by replacing these sheets (or pointing -hero at another hero.json), and
// the sheet format is the creatures' (data/strigoi/creatures): rows are
// directions in Diablo II's 8-direction order -- SW, NW, NE, SE, S, W, N, E
// -- columns are frames, the figure's feet 8 px above the frame's bottom
// centre.
//
//	go run ./tools/heroplaceholder            (from the repo root)
package main

import (
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

const (
	frameW, frameH = 96, 128
	feetX, feetY   = 48.0, 120.0
)

// Diablo II's 8-direction row order, as screen unit vectors (x right, y down).
var directions = [8][2]float64{
	{-1, 1}, {-1, -1}, {1, -1}, {1, 1}, // SW NW NE SE
	{0, 1}, {-1, 0}, {0, -1}, {1, 0}, // S W N E
}

type pt struct{ x, y float64 }

func fillPoly(img *image.NRGBA, ox int, poly []pt, c color.NRGBA) {
	minY, maxY := math.Inf(1), math.Inf(-1)
	for _, p := range poly {
		minY, maxY = math.Min(minY, p.y), math.Max(maxY, p.y)
	}

	for y := int(minY); y <= int(maxY)+1; y++ {
		for x := 0; x < frameW; x++ {
			if inside(poly, float64(x)+0.5, float64(y)+0.5) && y >= 0 && y < frameH {
				img.SetNRGBA(ox+x, y+0, c)
			}
		}
	}
}

func inside(poly []pt, x, y float64) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		a, b := poly[i], poly[j]
		if (a.y > y) != (b.y > y) && x < (b.x-a.x)*(y-a.y)/(b.y-a.y)+a.x {
			in = !in
		}
	}

	return in
}

func disc(img *image.NRGBA, ox int, cx, cy, r float64, c color.NRGBA) {
	for y := int(cy - r); y <= int(cy+r); y++ {
		for x := int(cx - r); x <= int(cx+r); x++ {
			if x >= 0 && x < frameW && y >= 0 && y < frameH && math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy) <= r {
				img.SetNRGBA(ox+x, y, c)
			}
		}
	}
}

func line(img *image.NRGBA, ox int, a, b pt, w float64, c color.NRGBA) {
	dx, dy := b.x-a.x, b.y-a.y
	n := math.Hypot(dx, dy)
	if n == 0 {
		return
	}

	px, py := -dy/n*w/2, dx/n*w/2
	fillPoly(img, ox, []pt{{a.x + px, a.y + py}, {b.x + px, b.y + py}, {b.x - px, b.y - py}, {a.x - px, a.y - py}}, c)
}

var (
	coat  = color.NRGBA{R: 58, G: 72, B: 104, A: 255}
	coatD = color.NRGBA{R: 40, G: 50, B: 74, A: 255}
	legs  = color.NRGBA{R: 92, G: 70, B: 50, A: 255}
	skin  = color.NRGBA{R: 196, G: 150, B: 112, A: 255}
	cap   = color.NRGBA{R: 236, G: 232, B: 220, A: 255}
	blade = color.NRGBA{R: 190, G: 196, B: 204, A: 255}
	blood = color.NRGBA{R: 150, G: 30, B: 24, A: 255}
)

// pose is one frame's figure.
type pose struct {
	bob    float64 // body raised (px)
	stride float64 // leg swing, -1..1
	swing  float64 // blade angle offset from facing, radians
	lean   float64 // lean back (px), for hit
	fall   float64 // 0 standing .. 1 lying
	hurt   bool
}

// figure draws one frame at column ox, facing dir.
func figure(img *image.NRGBA, ox int, dir [2]float64, p pose) {
	fx, fy := dir[0], dir[1]
	n := math.Hypot(fx, fy)
	fx, fy = fx/n, fy/n
	// On screen, "forward" squashes vertically (isometric): half the y.
	sx, sy := fx*1.0, fy*0.5

	if p.fall >= 1 {
		// Lying: the coat along the ground, the cap beside the head.
		body := []pt{{feetX - 26, feetY - 6}, {feetX + 22, feetY - 10}, {feetX + 24, feetY}, {feetX - 24, feetY + 2}}
		fillPoly(img, ox, body, coatD)
		disc(img, ox, feetX+28, feetY-6, 6, skin)
		fillPoly(img, ox, []pt{{feetX + 32, feetY - 14}, {feetX + 44, feetY - 18}, {feetX + 44, feetY - 8}, {feetX + 34, feetY - 6}}, cap)
		disc(img, ox, feetX-4, feetY+2, 5, blood)

		return
	}

	drop := p.fall * 40
	top := feetY - 64 + drop - p.bob
	lx := p.lean * -sx
	ly := p.lean * -sy

	// Legs, swinging along the facing.
	for _, side := range []float64{-1, 1} {
		foot := pt{feetX + side*5 + p.stride*side*sx*8, feetY + p.stride*side*sy*8}
		hip := pt{feetX + side*4 + lx, top + 40 + ly}
		line(img, ox, hip, foot, 5, legs)
	}

	// The coat: a long trapezoid, darker on the side facing away.
	col := coat
	if p.hurt {
		col = blood
	}

	fillPoly(img, ox, []pt{
		{feetX - 9 + lx, top + 12 + ly}, {feetX + 9 + lx, top + 12 + ly},
		{feetX + 13 + lx, top + 50 + ly}, {feetX - 13 + lx, top + 50 + ly},
	}, col)

	// The blade arm, forward along the facing, swung by the pose.
	ang := math.Atan2(sy, sx) + p.swing
	shoulder := pt{feetX + lx + sx*6, top + 18 + ly + sy*6}
	hand := pt{shoulder.x + math.Cos(ang)*12, shoulder.y + math.Sin(ang)*8}
	tip := pt{hand.x + math.Cos(ang)*24, hand.y + math.Sin(ang)*14 - 6}
	line(img, ox, shoulder, hand, 4, skin)
	line(img, ox, hand, tip, 2.5, blade)

	// Head, and the tall white cap folded back AWAY from the facing: the
	// silhouette's hook, and the placeholder's only reliable facing cue.
	hx, hy := feetX+lx+sx*2, top+6+ly
	disc(img, ox, hx, hy, 6, skin)
	fillPoly(img, ox, []pt{{hx - 6, hy - 3}, {hx + 6, hy - 3}, {hx + 4, hy - 22}, {hx - 4, hy - 22}}, cap)
	fillPoly(img, ox, []pt{{hx - 4, hy - 22}, {hx + 4, hy - 22}, {hx - sx*14 + 3, hy - 12 - sy*6}, {hx - sx*14 - 3, hy - 14 - sy*6}}, cap)
	// A dot of face toward the facing.
	disc(img, ox, hx+sx*4, hy+sy*3, 1.5, color.NRGBA{R: 60, G: 40, B: 30, A: 255})
}

type sheet struct {
	name   string
	frames []pose
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}

	dir := filepath.Join(root, "data", "strigoi", "hero", "placeholder")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		log.Fatal(err)
	}

	var sheets []sheet

	idle := sheet{name: "idle"}
	for i := 0; i < 4; i++ {
		idle.frames = append(idle.frames, pose{bob: math.Sin(float64(i)/4*2*math.Pi) * 1.5})
	}

	walk := sheet{name: "walk"}
	for i := 0; i < 8; i++ {
		walk.frames = append(walk.frames, pose{stride: math.Sin(float64(i) / 8 * 2 * math.Pi), bob: math.Abs(math.Cos(float64(i)/8*2*math.Pi)) * 2})
	}

	attack := sheet{name: "attack"}
	for i := 0; i < 6; i++ {
		attack.frames = append(attack.frames, pose{swing: -1.6 + float64(i)*0.55})
	}

	hit := sheet{name: "hit"}
	for i := 0; i < 3; i++ {
		hit.frames = append(hit.frames, pose{lean: float64(3 - i*1), hurt: i == 0})
	}

	death := sheet{name: "death"}
	for i := 0; i < 6; i++ {
		death.frames = append(death.frames, pose{fall: float64(i) / 6, lean: float64(i) * 2})
	}

	dead := sheet{name: "dead", frames: []pose{{fall: 1}}}

	sheets = append(sheets, idle, walk, attack, hit, death, dead)

	anims := map[string]string{}

	for _, s := range sheets {
		img := image.NewNRGBA(image.Rect(0, 0, frameW*len(s.frames), frameH*len(directions)))

		for row, d := range directions {
			sub := img.SubImage(image.Rect(0, row*frameH, img.Bounds().Dx(), (row+1)*frameH)).(*image.NRGBA)
			rowImg := image.NewNRGBA(image.Rect(0, 0, sub.Bounds().Dx(), frameH))

			for col, p := range s.frames {
				figure(rowImg, col*frameW, d, p)
			}

			for y := 0; y < frameH; y++ {
				copy(img.Pix[(row*frameH+y)*img.Stride:(row*frameH+y+1)*img.Stride], rowImg.Pix[y*rowImg.Stride:(y+1)*rowImg.Stride])
			}

			_ = sub
		}

		name := s.name + ".png"
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			log.Fatal(err)
		}

		if err := png.Encode(f, img); err != nil {
			log.Fatal(err)
		}

		f.Close()

		manifest, _ := json.MarshalIndent(map[string]any{
			"directions": len(directions), "frames_per_direction": len(s.frames),
			"frame_width": frameW, "frame_height": frameH,
			"offset_x": -frameW / 2, "offset_y": frameH - int(feetY),
			"origin_at_bottom": true,
		}, "", "  ")

		if err := os.WriteFile(filepath.Join(dir, name+".json"), append(manifest, '\n'), 0o640); err != nil {
			log.Fatal(err)
		}

		anims[s.name] = name
	}

	hero, _ := json.MarshalIndent(map[string]any{
		"name":       "Placeholder janissary (tools/heroplaceholder -- NOT the real art)",
		"animations": anims,
		// Quicker than the one-second default for the motions that should
		// snap; the walk and idle keep it.
		"fps": map[string]float64{"attack": 10, "hit": 10, "death": 8},
		// The stick figure stands from its feet (y 120) to the cap's top
		// (about y 31): the bar and label sit over the cap, not the cell.
		"height": 90,
	}, "", "  ")

	if err := os.WriteFile(filepath.Join(dir, "hero.json"), append(hero, '\n'), 0o640); err != nil {
		log.Fatal(err)
	}

	log.Printf("wrote %d sheets to %s", len(sheets), dir)
}
