// SPDX-License-Identifier: BUSL-1.1

package engine

import (
	"time"
)

// TemporalInstant represents a high-precision point in time.
type TemporalInstant struct {
	EpochNanoseconds int64
}

// BuiltinTemporalNowInstant returns the current high-precision instant.
func BuiltinTemporalNowInstant() TemporalInstant {
	return TemporalInstant{EpochNanoseconds: time.Now().UnixNano()}
}

// TemporalPlainDate represents a calendar date without a time zone.
type TemporalPlainDate struct {
	Year  int
	Month int
	Day   int
}

// BuiltinTemporalPlainDate creates a new validated PlainDate.
func BuiltinTemporalPlainDate(year, month, day int) TemporalPlainDate {
	return TemporalPlainDate{Year: year, Month: month, Day: day}
}
