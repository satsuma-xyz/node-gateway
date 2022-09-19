package route

import (
	"testing"

	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/metadata"
	"github.com/stretchr/testify/assert"
)

type AlwaysPass struct{}

func (AlwaysPass) Apply(metadata.RequestMetadata, *config.UpstreamConfig) bool {
	return true
}

type AlwaysFail struct{}

func (AlwaysFail) Apply(metadata.RequestMetadata, *config.UpstreamConfig) bool {
	return false
}

func TestAndFilter_Apply(t *testing.T) {
	type fields struct {
		filters []NodeFilter
	}

	type args struct {
		requestMetadata metadata.RequestMetadata
		upstreamConfig  *config.UpstreamConfig
	}

	tests := []struct { //nolint:govet // field alignment doesn't matter in tests
		name   string
		fields fields
		args   args
		want   bool
	}{
		{"No filters", fields{}, args{}, true},
		{"One passing filter", fields{[]NodeFilter{AlwaysPass{}}}, args{}, true},
		{"Multiple passing filters", fields{[]NodeFilter{AlwaysPass{}, AlwaysPass{}, AlwaysPass{}}}, args{}, true},
		{"One failing filter", fields{[]NodeFilter{AlwaysFail{}}}, args{}, false},
		{"Multiple passing and one failing", fields{[]NodeFilter{AlwaysPass{}, AlwaysPass{}, AlwaysFail{}}}, args{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &AndFilter{
				filters: tt.fields.filters,
			}
			assert.Equalf(t, tt.want, a.Apply(tt.args.requestMetadata, tt.args.upstreamConfig), "Apply(%v, %v)", tt.args.requestMetadata, tt.args.upstreamConfig)
		})
	}
}
