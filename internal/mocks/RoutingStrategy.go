// Code generated by mockery v2.43.2. DO NOT EDIT.

package mocks

import (
	metadata "github.com/satsuma-data/node-gateway/internal/metadata"
	mock "github.com/stretchr/testify/mock"

	types "github.com/satsuma-data/node-gateway/internal/types"
)

// MockRoutingStrategy is an autogenerated mock type for the RoutingStrategy type
type MockRoutingStrategy struct {
	mock.Mock
}

type MockRoutingStrategy_Expecter struct {
	mock *mock.Mock
}

func (_m *MockRoutingStrategy) EXPECT() *MockRoutingStrategy_Expecter {
	return &MockRoutingStrategy_Expecter{mock: &_m.Mock}
}

// RouteNextRequest provides a mock function with given fields: upstreamsByPriority, requestMetadata
func (_m *MockRoutingStrategy) RouteNextRequest(upstreamsByPriority types.PriorityToUpstreamsMap, requestMetadata metadata.RequestMetadata) (string, error) {
	ret := _m.Called(upstreamsByPriority, requestMetadata)

	if len(ret) == 0 {
		panic("no return value specified for RouteNextRequest")
	}

	var r0 string
	var r1 error
	if rf, ok := ret.Get(0).(func(types.PriorityToUpstreamsMap, metadata.RequestMetadata) (string, error)); ok {
		return rf(upstreamsByPriority, requestMetadata)
	}
	if rf, ok := ret.Get(0).(func(types.PriorityToUpstreamsMap, metadata.RequestMetadata) string); ok {
		r0 = rf(upstreamsByPriority, requestMetadata)
	} else {
		r0 = ret.Get(0).(string)
	}

	if rf, ok := ret.Get(1).(func(types.PriorityToUpstreamsMap, metadata.RequestMetadata) error); ok {
		r1 = rf(upstreamsByPriority, requestMetadata)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// MockRoutingStrategy_RouteNextRequest_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'RouteNextRequest'
type MockRoutingStrategy_RouteNextRequest_Call struct {
	*mock.Call
}

// RouteNextRequest is a helper method to define mock.On call
//   - upstreamsByPriority types.PriorityToUpstreamsMap
//   - requestMetadata metadata.RequestMetadata
func (_e *MockRoutingStrategy_Expecter) RouteNextRequest(upstreamsByPriority interface{}, requestMetadata interface{}) *MockRoutingStrategy_RouteNextRequest_Call {
	return &MockRoutingStrategy_RouteNextRequest_Call{Call: _e.mock.On("RouteNextRequest", upstreamsByPriority, requestMetadata)}
}

func (_c *MockRoutingStrategy_RouteNextRequest_Call) Run(run func(upstreamsByPriority types.PriorityToUpstreamsMap, requestMetadata metadata.RequestMetadata)) *MockRoutingStrategy_RouteNextRequest_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(types.PriorityToUpstreamsMap), args[1].(metadata.RequestMetadata))
	})
	return _c
}

func (_c *MockRoutingStrategy_RouteNextRequest_Call) Return(_a0 string, _a1 error) *MockRoutingStrategy_RouteNextRequest_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

func (_c *MockRoutingStrategy_RouteNextRequest_Call) RunAndReturn(run func(types.PriorityToUpstreamsMap, metadata.RequestMetadata) (string, error)) *MockRoutingStrategy_RouteNextRequest_Call {
	_c.Call.Return(run)
	return _c
}

// NewMockRoutingStrategy creates a new instance of MockRoutingStrategy. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewMockRoutingStrategy(t interface {
	mock.TestingT
	Cleanup(func())
}) *MockRoutingStrategy {
	mock := &MockRoutingStrategy{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
