package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	headPath = "M 128 64 C 140 64 148 66 154 70 L 196 40 Q 213 28 215 50 C 216 72 222 96 226 116 " +
		"C 230 136 240 176 204 208 C 184 226 156 230 128 230 C 100 230 72 226 52 208 " +
		"C 16 176 26 136 30 116 C 34 96 40 72 41 50 Q 43 28 60 40 L 102 70 C 108 66 116 64 128 64 Z"
	keyholeCX, keyholeCY, keyholeR = 128.0, 142.0, 21.0
	keyholeSlot                    = "M 119 155 L 137 155 L 143 193 Q 144 199 138 199 L 118 199 Q 112 199 113 193 Z"
)

var (
	headTop       = color.NRGBA{0xFF, 0xD3, 0x7A, 0xFF}
	headBottom    = color.NRGBA{0xF0, 0x9A, 0x22, 0xFF}
	privateTop    = color.NRGBA{0xC9, 0xA4, 0xFF, 0xFF}
	privateBottom = color.NRGBA{0x7B, 0x4B, 0xE0, 0xFF}
	tileTop       = color.NRGBA{0x22, 0x26, 0x2F, 0xFF}
	tileBottom    = color.NRGBA{0x0E, 0x10, 0x14, 0xFF}
)

type pt struct{ x, y float64 }

type transform struct{ s, dx, dy float64 }

func (t transform) apply(p pt) pt { return pt{p.x*t.s + t.dx, p.y*t.s + t.dy} }

func parsePath(d string, t transform) [][]pt {
	tokens := strings.Fields(strings.NewReplacer(",", " ").Replace(d))
	var out [][]pt
	var cur []pt
	var pos, start pt
	i := 0
	num := func() float64 {
		v, err := strconv.ParseFloat(tokens[i], 64)
		if err != nil {
			panic(err)
		}
		i++
		return v
	}
	cmd := ""
	for i < len(tokens) {
		if _, err := strconv.ParseFloat(tokens[i], 64); err != nil {
			cmd = tokens[i]
			i++
		}
		switch cmd {
		case "M":
			if len(cur) > 1 {
				out = append(out, cur)
			}
			pos = pt{num(), num()}
			start = pos
			cur = []pt{t.apply(pos)}
			cmd = "L"
		case "L":
			pos = pt{num(), num()}
			cur = append(cur, t.apply(pos))
		case "Q":
			c, e := pt{num(), num()}, pt{num(), num()}
			for k := 1; k <= 24; k++ {
				u := float64(k) / 24
				p := pt{
					(1-u)*(1-u)*pos.x + 2*(1-u)*u*c.x + u*u*e.x,
					(1-u)*(1-u)*pos.y + 2*(1-u)*u*c.y + u*u*e.y,
				}
				cur = append(cur, t.apply(p))
			}
			pos = e
		case "C":
			c1, c2, e := pt{num(), num()}, pt{num(), num()}, pt{num(), num()}
			for k := 1; k <= 48; k++ {
				u := float64(k) / 48
				a, b, c, dd := (1-u)*(1-u)*(1-u), 3*(1-u)*(1-u)*u, 3*(1-u)*u*u, u*u*u
				p := pt{a*pos.x + b*c1.x + c*c2.x + dd*e.x, a*pos.y + b*c1.y + c*c2.y + dd*e.y}
				cur = append(cur, t.apply(p))
			}
			pos = e
		case "Z":
			cur = append(cur, t.apply(start))
			out = append(out, cur)
			cur = nil
			pos = start
		default:
			panic("unsupported path command " + cmd)
		}
	}
	if len(cur) > 1 {
		out = append(out, cur)
	}
	return out
}

func circle(cx, cy, r float64, t transform) [][]pt {
	var poly []pt
	for k := 0; k <= 128; k++ {
		a := 2 * math.Pi * float64(k) / 128
		poly = append(poly, t.apply(pt{cx + r*math.Cos(a), cy + r*math.Sin(a)}))
	}
	return [][]pt{poly}
}

func roundedRect(x, y, w, h, r float64) [][]pt {
	var poly []pt
	corner := func(cx, cy, from float64) {
		for k := 0; k <= 32; k++ {
			a := from + math.Pi/2*float64(k)/32
			poly = append(poly, pt{cx + r*math.Cos(a), cy + r*math.Sin(a)})
		}
	}
	corner(x+w-r, y+r, -math.Pi/2)
	corner(x+w-r, y+h-r, 0)
	corner(x+r, y+h-r, math.Pi/2)
	corner(x+r, y+r, math.Pi)
	poly = append(poly, poly[0])
	return [][]pt{poly}
}

func coverage(polys [][]pt, w, h int) []float64 {
	const sub = 16
	cov := make([]float64, w*h)
	type cross struct {
		x   float64
		dir int
	}
	var xs []cross
	for py := 0; py < h; py++ {
		for s := 0; s < sub; s++ {
			y := float64(py) + (float64(s)+0.5)/sub
			xs = xs[:0]
			for _, poly := range polys {
				for i := 0; i+1 < len(poly); i++ {
					a, b := poly[i], poly[i+1]
					if a.y == b.y {
						continue
					}
					dir := 1
					if a.y > b.y {
						a, b = b, a
						dir = -1
					}
					if y < a.y || y >= b.y {
						continue
					}
					xs = append(xs, cross{a.x + (y-a.y)*(b.x-a.x)/(b.y-a.y), dir})
				}
			}
			sort.Slice(xs, func(i, j int) bool { return xs[i].x < xs[j].x })
			wind := 0
			for i := 0; i+1 < len(xs); i++ {
				wind += xs[i].dir
				if wind == 0 {
					continue
				}
				x0, x1 := math.Max(xs[i].x, 0), math.Min(xs[i+1].x, float64(w))
				for px := int(x0); px < w && float64(px) < x1; px++ {
					l, r := math.Max(x0, float64(px)), math.Min(x1, float64(px+1))
					if r > l {
						cov[py*w+px] += (r - l) / sub
					}
				}
			}
		}
	}
	for i := range cov {
		cov[i] = math.Min(cov[i], 1)
	}
	return cov
}

func lerp(a, b color.NRGBA, t float64) [3]float64 {
	t = math.Max(0, math.Min(1, t))
	return [3]float64{
		float64(a.R) + (float64(b.R)-float64(a.R))*t,
		float64(a.G) + (float64(b.G)-float64(a.G))*t,
		float64(a.B) + (float64(b.B)-float64(a.B))*t,
	}
}

func render(size int, tile bool) *image.NRGBA {
	return renderColors(size, tile, headTop, headBottom)
}

func renderColors(size int, tile bool, headTop, headBottom color.NRGBA) *image.NRGBA {
	n := float64(size)
	var glyph transform
	var tileCov []float64
	fill, holeBoost := 0.72, 1.0
	switch {
	case size <= 24:
		fill, holeBoost = 0.90, 1.3
	case size <= 40:
		fill, holeBoost = 0.80, 1.15
	}
	if tile {
		s := n / 256 * fill
		glyph = transform{s, n/2 - 128*s, n/2 - 135*s}
		r := n * 0.225
		tileCov = coverage(roundedRect(0, 0, n, n, r), size, size)
	} else {
		s := n / 256 * 1.02
		glyph = transform{s, n/2 - 128*s, n/2 - 133*s}
	}
	const holeCX, holeCY = 128.0, 160.0
	hole := transform{holeBoost * glyph.s, glyph.dx + glyph.s*holeCX*(1-holeBoost), glyph.dy + glyph.s*holeCY*(1-holeBoost)}
	head := coverage(parsePath(headPath, glyph), size, size)
	holeCov := coverage(append(circle(keyholeCX, keyholeCY, keyholeR, hole), parsePath(keyholeSlot, hole)...), size, size)
	headY0, headY1 := glyph.apply(pt{0, 30}).y, glyph.apply(pt{0, 230}).y

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		hc := lerp(headTop, headBottom, (float64(y)+0.5-headY0)/(headY1-headY0))
		tc := lerp(tileTop, tileBottom, (float64(y)+0.5)/n)
		for x := 0; x < size; x++ {
			i := y*size + x
			fa := head[i] * (1 - holeCov[i])
			ba := 0.0
			if tile {
				ba = tileCov[i]
			}
			outA := fa + ba*(1-fa)
			var c [3]float64
			if outA > 0 {
				for k := 0; k < 3; k++ {
					c[k] = (hc[k]*fa + tc[k]*ba*(1-fa)) / outA
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{uint8(math.Round(c[0])), uint8(math.Round(c[1])), uint8(math.Round(c[2])), uint8(math.Round(outA * 255))})
		}
	}
	return img
}

func encodePNG(img image.Image) []byte {
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}

func writeFile(path string, data []byte) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		panic(err)
	}
	fmt.Println("wrote", path)
}

func ico(sizes []int, tile bool) []byte {
	var images [][]byte
	for _, s := range sizes {
		images = append(images, encodePNG(render(s, tile)))
	}
	var buf bytes.Buffer
	binary.Write(&buf, binary.LittleEndian, [3]uint16{0, 1, uint16(len(sizes))})
	offset := 6 + 16*len(sizes)
	for i, s := range sizes {
		dim := uint8(s)
		if s >= 256 {
			dim = 0
		}
		buf.Write([]byte{dim, dim, 0, 0})
		binary.Write(&buf, binary.LittleEndian, uint16(1))
		binary.Write(&buf, binary.LittleEndian, uint16(32))
		binary.Write(&buf, binary.LittleEndian, uint32(len(images[i])))
		binary.Write(&buf, binary.LittleEndian, uint32(offset))
		offset += len(images[i])
	}
	for _, data := range images {
		buf.Write(data)
	}
	return buf.Bytes()
}

func letterbox(w, h, glyph int) *image.NRGBA {
	return letterboxColors(w, h, glyph, headTop, headBottom)
}

func letterboxColors(w, h, glyph int, top, bottom color.NRGBA) *image.NRGBA {
	src := renderColors(glyph, false, top, bottom)
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	ox, oy := (w-glyph)/2, (h-glyph)/2
	for y := 0; y < glyph; y++ {
		for x := 0; x < glyph; x++ {
			img.SetNRGBA(x+ox, y+oy, src.NRGBAAt(x, y))
		}
	}
	return img
}

func main() {
	root := "."
	if len(os.Args) > 1 {
		root = os.Args[1]
	}
	brand := filepath.Join(root, "browser", "branding", "omni", "chrome", "browser", "content", "branding")
	for _, s := range []int{16, 32, 48, 64, 128} {
		writeFile(filepath.Join(brand, fmt.Sprintf("icon%d.png", s)), encodePNG(render(s, true)))
	}
	writeFile(filepath.Join(brand, "about-logo.png"), encodePNG(render(192, false)))
	writeFile(filepath.Join(brand, "about-logo@2x.png"), encodePNG(render(384, false)))
	writeFile(filepath.Join(brand, "about-logo-private.png"), encodePNG(render(192, false)))
	writeFile(filepath.Join(brand, "about-logo-private@2x.png"), encodePNG(render(384, false)))
	writeFile(filepath.Join(brand, "about.png"), encodePNG(letterbox(300, 236, 200)))

	sizes := []int{16, 20, 24, 32, 40, 48, 64, 96, 128, 256}
	appIcon := ico(sizes, true)
	writeFile(filepath.Join(brand, "document.ico"), appIcon)
	icons := filepath.Join(root, "browser", "branding", "icons")
	writeFile(filepath.Join(icons, "main-window.ico"), appIcon)
	writeFile(filepath.Join(icons, "default.ico"), appIcon)
	writeFile(filepath.Join(root, "cmd", "keycat", "keycat.ico"), appIcon)
	for _, s := range []int{16, 32, 48, 64, 128} {
		writeFile(filepath.Join(icons, fmt.Sprintf("default%d.png", s)), encodePNG(render(s, true)))
	}
	for _, s := range []int{16, 24, 32, 48, 64, 128, 256, 512} {
		writeFile(filepath.Join(root, "linux", "icons", fmt.Sprintf("keycat-%d.png", s)), encodePNG(render(s, true)))
	}

	art := filepath.Join(root, "browser", "branding", "art")
	writeFile(filepath.Join(art, "logo.png"), encodePNG(render(256, false)))
	writeFile(filepath.Join(art, "illustration.png"), encodePNG(letterbox(320, 320, 240)))
	writeFile(filepath.Join(art, "private.png"), encodePNG(renderColors(256, false, privateTop, privateBottom)))
	writeFile(filepath.Join(art, "glyph.png"), encodePNG(render(256, false)))
	writeFile(filepath.Join(art, "icon.ico"), appIcon)

	visual := filepath.Join(root, "browser", "core", "browser", "VisualElements")
	for _, s := range []int{70, 150} {
		writeFile(filepath.Join(visual, fmt.Sprintf("VisualElements_%d.png", s)), encodePNG(render(s, true)))
		writeFile(filepath.Join(visual, fmt.Sprintf("PrivateBrowsing_%d.png", s)), encodePNG(render(s, true)))
	}

	res := filepath.Join(root, "android", "app", "src", "main", "res")
	white := color.NRGBA{0xFF, 0xFF, 0xFF, 0xFF}
	for dpi, k := range map[string]float64{"mdpi": 1, "hdpi": 1.5, "xhdpi": 2, "xxhdpi": 3, "xxxhdpi": 4} {
		canvas, glyph := int(108*k), int(58*k)
		writeFile(filepath.Join(res, "mipmap-"+dpi, "ic_launcher_foreground.png"), encodePNG(letterbox(canvas, canvas, glyph)))
		writeFile(filepath.Join(res, "mipmap-"+dpi, "ic_launcher_monochrome.png"), encodePNG(letterboxColors(canvas, canvas, glyph, white, white)))
		writeFile(filepath.Join(res, "drawable-"+dpi, "ic_stat_keycat.png"), encodePNG(renderColors(int(24*k), false, white, white)))
	}
	writeFile(filepath.Join(res, "drawable-nodpi", "keycat_logo.png"), encodePNG(render(384, true)))

	assets := filepath.Join(root, "assets", "brand")
	writeFile(filepath.Join(assets, "keycat-1024.png"), encodePNG(render(1024, true)))
	writeFile(filepath.Join(assets, "keycat-glyph-512.png"), encodePNG(render(512, false)))
	writeFile(filepath.Join(root, "hub", "img", "keycat-192.png"), encodePNG(render(192, true)))
}
