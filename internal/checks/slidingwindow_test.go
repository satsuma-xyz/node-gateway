package checks

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func Test_SimpleSlidingWindow(t *testing.T) {
	Assert := assert.New(t)

	w := NewSimpleSlidingWindow(time.Minute)
	Assert.Equal(0, w.Count())
	Assert.Equal(time.Duration(0), w.Sum())
	Assert.Equal(time.Duration(0), w.Mean())

	w.AddValue(time.Second)
	Assert.Equal(1, w.Count())
	Assert.Equal(time.Second, w.Sum())
	Assert.Equal(time.Second, w.Mean())

	w.AddValue(3 * time.Second)
	Assert.Equal(2, w.Count())
	Assert.Equal(4*time.Second, w.Sum())
	Assert.Equal(2*time.Second, w.Mean())

	w.AddValue(6 * time.Second)
	Assert.Equal(3, w.Count())
	Assert.Equal(10*time.Second, w.Sum())
	Assert.Equal(10*time.Second/3, w.Mean())
}
