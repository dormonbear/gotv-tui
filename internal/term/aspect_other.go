//go:build !unix

package term

func measuredAspect() (float64, bool) { return 0, false }

func cellPixels() (float64, float64, bool) { return 0, 0, false }
