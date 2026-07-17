package main

import "math"

func validPriority(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func validPriority32(value float32) bool {
	return validPriority(float64(value))
}
