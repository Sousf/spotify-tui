package viz

import "math"

// fft is an in-place iterative radix-2 Cooley-Tukey transform. len(re) must
// be a power of two and im is expected to be zeroed by the caller.
func fft(re, im []float64) {
	n := len(re)
	// Bit-reversal permutation.
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	for size := 2; size <= n; size <<= 1 {
		ang := -2 * math.Pi / float64(size)
		wr, wi := math.Cos(ang), math.Sin(ang)
		for start := 0; start < n; start += size {
			cr, ci := 1.0, 0.0
			half := size / 2
			for k := 0; k < half; k++ {
				a, b := start+k, start+k+half
				tr := re[b]*cr - im[b]*ci
				ti := re[b]*ci + im[b]*cr
				re[b], im[b] = re[a]-tr, im[a]-ti
				re[a] += tr
				im[a] += ti
				cr, ci = cr*wr-ci*wi, cr*wi+ci*wr
			}
		}
	}
}
