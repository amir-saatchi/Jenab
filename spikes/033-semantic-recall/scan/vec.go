package main

import (
	"math"
	"math/rand"
)

// unit returns a random unit vector (Gaussian direction, so uniform on the sphere).
func unit(r *rand.Rand, w int) []float32 {
	v := make([]float32, w)
	var s float64
	for i := range v {
		x := r.NormFloat64()
		v[i] = float32(x)
		s += x * x
	}
	inv := float32(1 / math.Sqrt(s))
	for i := range v {
		v[i] *= inv
	}
	return v
}

// quantize is symmetric per-vector int8: q = round(x/scale), scale = max|x|/127.
func quantize(v []float32, dst []int8) (scale float32) {
	var m float32
	for _, x := range v {
		if x < 0 {
			x = -x
		}
		if x > m {
			m = x
		}
	}
	scale = m / 127
	for i, x := range v {
		dst[i] = int8(math.Round(float64(x / scale)))
	}
	return scale
}

func dotF32(a, b []float32) float32 {
	var s0, s1, s2, s3 float32
	b = b[:len(a)]
	for i := 0; i+4 <= len(a); i += 4 {
		s0 += a[i] * b[i]
		s1 += a[i+1] * b[i+1]
		s2 += a[i+2] * b[i+2]
		s3 += a[i+3] * b[i+3]
	}
	return s0 + s1 + s2 + s3
}

func dotI8(a, b []int8) int32 {
	var s0, s1, s2, s3 int32
	b = b[:len(a)]
	for i := 0; i+4 <= len(a); i += 4 {
		s0 += int32(a[i]) * int32(b[i])
		s1 += int32(a[i+1]) * int32(b[i+1])
		s2 += int32(a[i+2]) * int32(b[i+2])
		s3 += int32(a[i+3]) * int32(b[i+3])
	}
	return s0 + s1 + s2 + s3
}
