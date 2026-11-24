//go:build !unix

package term

func measuredAspect() (float64, bool) { return 0, false }
