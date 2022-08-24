package route

import (
	"io"
	"sync"
	"testing"

	"github.com/satsuma-data/node-gateway/internal/config"
	"github.com/satsuma-data/node-gateway/internal/jsonrpc"
	"github.com/satsuma-data/node-gateway/internal/mocks"
	"github.com/stretchr/testify/assert"
)

func TestRouter_NoHealthyUpstreams(t *testing.T) {
	managerMock := mocks.NewHealthCheckManager(t)
	managerMock.On("GetHealthyUpstreams").Return([]string{})

	router := SimpleRouter{
		healthCheckManager: managerMock,
		upstreamsMutex:     &sync.RWMutex{},
		routingStrategy:    NewPriorityRoundRobinStrategy(),
	}

	jsonResp, httpResp, err := router.Route(jsonrpc.RequestBody{})
	defer httpResp.Body.Close()

	assert.Nil(t, jsonResp)
	assert.Equal(t, 503, httpResp.StatusCode)
	assert.Equal(t, "No healthy upstream", readyBody(httpResp.Body))
	assert.Nil(t, err)
}

func readyBody(body io.ReadCloser) string {
	bodyBytes, _ := io.ReadAll(body)
	return string(bodyBytes)
}

func TestGroupUpstreamsByPriority(t *testing.T) {
	upstreams := []string{"geth", "erigon", "openethereum"}
	upstreamConfigs := map[string]config.UpstreamConfig{
		"geth":           {GroupID: "primary"},
		"erigon":         {GroupID: "fallback"},
		"openethereum":   {GroupID: "backup"},
		"something-else": {GroupID: "backup"},
	}
	groupConfigs := map[string]config.GroupConfig{
		"primary":  {Priority: 0},
		"fallback": {Priority: 1},
		"backup":   {Priority: 2},
	}

	upstreamsByPriority := groupUpstreamsByPriority(upstreams, upstreamConfigs, groupConfigs)
	expectedPriorityMap := map[int][]string{
		0: {"geth"},
		1: {"erigon"},
		2: {"openethereum"},
	}
	assert.Equal(t, expectedPriorityMap, upstreamsByPriority)
}

func TestGroupUpstreamsByPriority_NoGroups(t *testing.T) {
	upstreams := []string{"geth", "erigon", "openethereum"}
	upstreamConfigs := map[string]config.UpstreamConfig{
		"geth":           {},
		"erigon":         {},
		"openethereum":   {},
		"something-else": {},
	}
	groupConfigs := make(map[string]config.GroupConfig)

	// Verify they are all on the same priority
	upstreamsByPriority := groupUpstreamsByPriority(upstreams, upstreamConfigs, groupConfigs)
	expectedPriorityMap := map[int][]string{
		0: {"geth", "erigon", "openethereum"},
	}
	assert.Equal(t, expectedPriorityMap, upstreamsByPriority)
}
