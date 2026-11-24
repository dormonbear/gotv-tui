package radar

import (
	"image"
	"math"
)

// Box is a crop in 1024x1024 radar space.
type Box struct{ X, Y, W, H float64 }

// Thin walls are a few source pixels wide; render at SS x the dot grid so they
// survive and can be traced before downsampling.
const SS = 4

// Raster is a cropped radar scaled to a braille dot grid (one cell = 2x4 dots).
type Raster struct {
	W, H   int     // dots
	Pixels []byte  // RGBA, (W*SS) x (H*SS)
	Aspect float64 // terminal cell height / width
	ov     Overview
	box    Box
	k      float64 // horizontal dots per radar pixel
	ky     float64 // vertical dots per radar pixel (cells aren't always 1:2)
}

// ToPx maps world x/y to dot coordinates.
func (r *Raster) ToPx(x, y float64) (float64, float64) {
	return ((x-r.ov.PosX)/r.ov.Scale - r.box.X) * r.k, ((r.ov.PosY-y)/r.ov.Scale - r.box.Y) * r.ky
}

// NormPx maps 0..1 radar coords to dot coordinates.
func (r *Raster) NormPx(nx, ny float64) (float64, float64) {
	return (nx*1024 - r.box.X) * r.k, (ny*1024 - r.box.Y) * r.ky
}

// IsFloor: opaque and not near-black (lower-level radars ship a solid dark backdrop).
func IsFloor(p []byte, i int) bool {
	return p[i*4+3] > 40 && float64(p[i*4])*0.3+float64(p[i*4+1])*0.59+float64(p[i*4+2])*0.11 >= 40
}

// Resample area-averages box (1024 radar space) of img into outW x outH RGBA pixels.
func Resample(img *image.NRGBA, box Box, outW, outH int) []byte {
	out := make([]byte, outW*outH*4)
	iw, ih := img.Rect.Dx(), img.Rect.Dy()
	sx, sy := box.W/float64(outW)*float64(iw)/1024, box.H/float64(outH)*float64(ih)/1024
	ox, oy := box.X*float64(iw)/1024, box.Y*float64(ih)/1024
	for y := 0; y < outH; y++ {
		y0 := max(0, int(math.Floor(oy+float64(y)*sy)))
		y1 := min(ih, max(y0+1, int(math.Floor(oy+float64(y+1)*sy))))
		for x := 0; x < outW; x++ {
			x0 := max(0, int(math.Floor(ox+float64(x)*sx)))
			x1 := min(iw, max(x0+1, int(math.Floor(ox+float64(x+1)*sx))))
			var r, g, b, a float64
			n := 0
			for yy := y0; yy < y1; yy++ {
				row := img.Pix[yy*img.Stride:]
				for xx := x0; xx < x1; xx++ {
					px := row[xx*4:]
					al := float64(px[3])
					r, g, b, a = r+float64(px[0])*al, g+float64(px[1])*al, b+float64(px[2])*al, a+al
					n++
				}
			}
			o := (y*outW + x) * 4
			if a > 0 {
				out[o], out[o+1], out[o+2] = byte(r/a), byte(g/a), byte(b/a)
			}
			if n > 0 {
				out[o+3] = byte(a / float64(n))
			}
		}
	}
	return out
}

// OpaqueBox is the bounding box of the floor of all images (radars carry wide empty
// margins). Sparse rows/columns (< 8% of the densest) are ignored so specks don't widen it.
func OpaqueBox(imgs ...*image.NRGBA) Box {
	const n, edge = 256, 8
	rows, cols := make([]int, n), make([]int, n)
	for _, img := range imgs {
		if img == nil {
			continue
		}
		px := Resample(img, Box{0, 0, 1024, 1024}, n, n)
		// skip the outer margin: some radars (nuke lower) have a frame line around the image
		for y := edge; y < n-edge; y++ {
			for x := edge; x < n-edge; x++ {
				if IsFloor(px, y*n+x) {
					rows[y]++
					cols[x]++
				}
			}
		}
	}
	span := func(a []int) (int, int) {
		top := 0
		for _, v := range a {
			top = max(top, v)
		}
		cut := max(6, int(float64(top)*0.08))
		lo, hi := -1, -1
		for i, v := range a {
			if v >= cut {
				if lo < 0 {
					lo = i
				}
				hi = i
			}
		}
		return lo, hi
	}
	x0, x1 := span(cols)
	y0, y1 := span(rows)
	if x0 < 0 || y0 < 0 {
		return Box{0, 0, 1024, 1024}
	}
	k := 1024.0 / n
	return Box{float64(x0-1) * k, float64(y0-1) * k, float64(x1-x0+3) * k, float64(y1-y0+3) * k}
}

// FitScale is the horizontal dots per radar pixel that fits box into maxW x maxH dots
// on cells of the given aspect (height / width; a braille dot is 1/2 x 1/4 cell).
func FitScale(box Box, maxW, maxH int, aspect float64) float64 {
	return min(float64(maxW)/box.W, float64(maxH)*aspect/2/box.H)
}

// Rasterize renders the cropped radar to fit maxW x maxH dots, keeping the map's
// proportions on screen for the terminal's cell aspect.
func Rasterize(img *image.NRGBA, ov Overview, box Box, maxW, maxH int, aspect float64) *Raster {
	k := FitScale(box, maxW, maxH, aspect)
	w := max(8, int(k*box.W))
	k = float64(w) / box.W
	ky := k * 2 / aspect
	h := max(1, int(math.Round(box.H*ky)))
	return &Raster{W: w, H: h, Pixels: Resample(img, box, w*SS, h*SS), Aspect: aspect, ov: ov, box: box, k: k, ky: ky}
}

// MaxZoom: past ~4x a braille dot is finer than a radar pixel and only blurs.
const MaxZoom = 4.0

// View is the part of b shown at zoom z around (cx, cy), both in 1024 radar space.
// It keeps b's shape (so the framed map keeps its size) and stays inside b.
func (b Box) View(cx, cy, z float64) Box {
	z = min(MaxZoom, max(1, z))
	w, h := b.W/z, b.H/z
	x := min(max(cx-w/2, b.X), b.X+b.W-w)
	y := min(max(cy-h/2, b.Y), b.Y+b.H-h)
	return Box{x, y, w, h}
}

// Center is the middle of b.
func (b Box) Center() (float64, float64) { return b.X + b.W/2, b.Y + b.H/2 }
