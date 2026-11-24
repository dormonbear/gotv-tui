package term

import (
	"os"
	"strconv"
)

// DefaultAspect is the usual terminal cell height / width.
const DefaultAspect = 2.0

// CellAspect is the cell height / width of the terminal on stdout: GOTV_TUI_CELL_ASPECT
// if set, else measured from the window's pixel size, else DefaultAspect.
func CellAspect() float64 {
	if v, err := strconv.ParseFloat(os.Getenv("GOTV_TUI_CELL_ASPECT"), 64); err == nil && v >= 1 && v <= 4 {
		return v
	}
	if a, ok := measuredAspect(); ok && a >= 1 && a <= 4 {
		return a
	}
	return DefaultAspect
}
