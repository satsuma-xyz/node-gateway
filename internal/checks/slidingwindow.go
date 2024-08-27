package checks

import "time"

// SlidingWindow represents a container of timestamped duration values.
// Values that are older than a threshold are discarded.
// TODO(polsar): Use a generic type parameter T instead of `time.Duration`: https://go.dev/doc/tutorial/generics
type SlidingWindow interface {
	// AddValue adds the specified value to the sliding window at the current timestamp.
	AddValue(v time.Duration)

	// Count returns the current number of values in the sliding window.
	Count() int

	// Sum returns the current sum of the values in the sliding window.
	Sum() time.Duration

	// Mean returns the current sum of the values in the sliding window divided by the number of values.
	Mean() time.Duration
}

type timestampedValue struct {
	ts    time.Time
	value time.Duration
}

type SimpleSlidingWindow struct {
	values   []timestampedValue
	duration time.Duration
	sum      time.Duration
}

func NewSimpleSlidingWindow(duration time.Duration) SlidingWindow {
	return &SimpleSlidingWindow{duration: duration}
}

func (s *SimpleSlidingWindow) AddValue(v time.Duration) {
	now := time.Now()
	// TODO(polsar): Drop old value(s) and subtract from the running sum.
	s.values = append(s.values, timestampedValue{value: v, ts: now})
	s.sum += v
}

func (s *SimpleSlidingWindow) Count() int {
	return len(s.values)
}

func (s *SimpleSlidingWindow) Sum() time.Duration {
	return s.sum
}

func (s *SimpleSlidingWindow) Mean() time.Duration {
	c := s.Count()

	if c == 0 {
		return 0
	}

	return time.Duration(s.Sum().Nanoseconds()/int64(c)) * time.Nanosecond
}
