package route

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_GetFilterTypeName(t *testing.T) {
	Assert := assert.New(t)

	Assert.Equal(
		NodeFilterType("AlwaysPass"),
		GetFilterTypeName(AlwaysPass{}),
	)
	Assert.Equal(
		NodeFilterType("AlwaysFail"),
		GetFilterTypeName(AlwaysFail{}),
	)
	Assert.Equal(
		NodeFilterType("AndFilter"),
		GetFilterTypeName(&AndFilter{}),
	)
}

func Test_RemoveFilters_RemoveNone(t *testing.T) {
	Assert := assert.New(t)

	Assert.Equal(
		[]NodeFilter{},
		removeFilters([]NodeFilter{}, "AlwaysPass"),
	)
	Assert.Equal(
		[]NodeFilter{AlwaysFail{}},
		removeFilters([]NodeFilter{AlwaysFail{}}, "AlwaysPass"),
	)
	Assert.Equal(
		[]NodeFilter{AlwaysFail{}, &AndFilter{}},
		removeFilters([]NodeFilter{AlwaysFail{}, &AndFilter{}}, "AlwaysPass"),
	)
}

func Test_RemoveFilters_RemoveOne(t *testing.T) {
	Assert := assert.New(t)

	Assert.Equal(
		[]NodeFilter{},
		removeFilters([]NodeFilter{AlwaysPass{}}, "AlwaysPass"),
	)
	Assert.Equal(
		[]NodeFilter{AlwaysFail{}},
		removeFilters([]NodeFilter{AlwaysFail{}, AlwaysPass{}}, "AlwaysPass"),
	)
	Assert.Equal(
		[]NodeFilter{&AndFilter{}},
		removeFilters([]NodeFilter{AlwaysPass{}, &AndFilter{}}, "AlwaysPass"),
	)
	Assert.Equal(
		[]NodeFilter{AlwaysPass{}},
		removeFilters([]NodeFilter{AlwaysPass{}, &AndFilter{}}, "AndFilter"),
	)
}

func Test_RemoveFilters_RemoveTwo(t *testing.T) {
	Assert := assert.New(t)

	Assert.Equal(
		[]NodeFilter{},
		removeFilters([]NodeFilter{AlwaysPass{}, AlwaysPass{}}, "AlwaysPass"),
	)
	Assert.Equal(
		[]NodeFilter{&AndFilter{}},
		removeFilters([]NodeFilter{AlwaysPass{}, &AndFilter{}, AlwaysPass{}}, "AlwaysPass"),
	)
	Assert.Equal(
		[]NodeFilter{AlwaysPass{}},
		removeFilters([]NodeFilter{AlwaysPass{}, &AndFilter{}, &AndFilter{}}, "AndFilter"),
	)
	Assert.Equal(
		[]NodeFilter{AlwaysFail{}},
		removeFilters([]NodeFilter{AlwaysPass{}, AlwaysPass{}, AlwaysFail{}}, "AlwaysPass"),
	)
}
