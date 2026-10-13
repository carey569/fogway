//go:build darwin && cgo

package gui

import (
	"image"
)

// inkIn counts the pixels in r drawn in the bar's text colour.
func inkIn(img image.Image, r image.Rectangle, dark bool) int {
	n := 0
	for y := r.Min.Y; y < r.Max.Y && y < img.Bounds().Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X && x < img.Bounds().Max.X; x++ {
			c, g, b, a := img.At(x, y).RGBA()
			if a < 0x8000 {
				continue
			}
			lum := (c + g + b) / 3 * 0xffff / max(a, 1)
			if dark && lum > 0xc000 || !dark && lum < 0x4000 {
				n++
			}
		}
	}
	return n
}
