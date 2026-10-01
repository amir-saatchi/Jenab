package main

import "time"

// between converts two QPC stamps into a duration.
func between(a, b stamp) time.Duration {
	return time.Duration(float64(b-a) * 1e9 / float64(qpcFreq))
}
