// Code generated by mockery v2.14.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// RoutingStrategy is an autogenerated mock type for the RoutingStrategy type
type RoutingStrategy struct {
	mock.Mock
}

type RoutingStrategy_Expecter struct {
	mock *mock.Mock
}

func (_m *RoutingStrategy) EXPECT() *RoutingStrategy_Expecter {
	return &RoutingStrategy_Expecter{mock: &_m.Mock}
}

// RouteNextRequest provides a mock function with given fields: upstreamsByPriority
func (_m *RoutingStrategy) RouteNextRequest(upstreamsByPriority map[int][]string) (string, error) {
	ret := _m.Called(upstreamsByPriority)

	var r0 string
	if rf, ok := ret.Get(0).(func(map[int][]string) string); ok {
		r0 = rf(upstreamsByPriority)
	} else {
		r0 = ret.Get(0).(string)
	}

	var r1 error
	if rf, ok := ret.Get(1).(func(map[int][]string) error); ok {
		r1 = rf(upstreamsByPriority)
	} else {
		r1 = ret.Error(1)
	}

	return r0, r1
}

// RoutingStrategy_RouteNextRequest_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'RouteNextRequest'
type RoutingStrategy_RouteNextRequest_Call struct {
	*mock.Call
}

// RouteNextRequest is a helper method to define mock.On call
//  - upstreamsByPriority map[int][]string
func (_e *RoutingStrategy_Expecter) RouteNextRequest(upstreamsByPriority interface{}) *RoutingStrategy_RouteNextRequest_Call {
	return &RoutingStrategy_RouteNextRequest_Call{Call: _e.mock.On("RouteNextRequest", upstreamsByPriority)}
}

func (_c *RoutingStrategy_RouteNextRequest_Call) Run(run func(upstreamsByPriority map[int][]string)) *RoutingStrategy_RouteNextRequest_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(map[int][]string))
	})
	return _c
}

func (_c *RoutingStrategy_RouteNextRequest_Call) Return(_a0 string, _a1 error) *RoutingStrategy_RouteNextRequest_Call {
	_c.Call.Return(_a0, _a1)
	return _c
}

type mockConstructorTestingTNewRoutingStrategy interface {
	mock.TestingT
	Cleanup(func())
}

// NewRoutingStrategy creates a new instance of RoutingStrategy. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewRoutingStrategy(t mockConstructorTestingTNewRoutingStrategy) *RoutingStrategy {
	mock := &RoutingStrategy{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
