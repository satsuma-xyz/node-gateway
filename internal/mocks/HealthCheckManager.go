// Code generated by mockery v2.14.0. DO NOT EDIT.

package mocks

import (
	types "github.com/satsuma-data/node-gateway/internal/types"
	mock "github.com/stretchr/testify/mock"
)

// HealthCheckManager is an autogenerated mock type for the HealthCheckManager type
type HealthCheckManager struct {
	mock.Mock
}

type HealthCheckManager_Expecter struct {
	mock *mock.Mock
}

func (_m *HealthCheckManager) EXPECT() *HealthCheckManager_Expecter {
	return &HealthCheckManager_Expecter{mock: &_m.Mock}
}

// GetUpstreamStatus provides a mock function with given fields: upstreamID
func (_m *HealthCheckManager) GetUpstreamStatus(upstreamID string) *types.UpstreamStatus {
	ret := _m.Called(upstreamID)

	var r0 *types.UpstreamStatus
	if rf, ok := ret.Get(0).(func(string) *types.UpstreamStatus); ok {
		r0 = rf(upstreamID)
	} else {
		if ret.Get(0) != nil {
			r0 = ret.Get(0).(*types.UpstreamStatus)
		}
	}

	return r0
}

// HealthCheckManager_GetUpstreamStatus_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetUpstreamStatus'
type HealthCheckManager_GetUpstreamStatus_Call struct {
	*mock.Call
}

// GetUpstreamStatus is a helper method to define mock.On call
//   - upstreamID string
func (_e *HealthCheckManager_Expecter) GetUpstreamStatus(upstreamID interface{}) *HealthCheckManager_GetUpstreamStatus_Call {
	return &HealthCheckManager_GetUpstreamStatus_Call{Call: _e.mock.On("GetUpstreamStatus", upstreamID)}
}

func (_c *HealthCheckManager_GetUpstreamStatus_Call) Run(run func(upstreamID string)) *HealthCheckManager_GetUpstreamStatus_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(string))
	})
	return _c
}

func (_c *HealthCheckManager_GetUpstreamStatus_Call) Return(_a0 *types.UpstreamStatus) *HealthCheckManager_GetUpstreamStatus_Call {
	_c.Call.Return(_a0)
	return _c
}

// StartHealthChecks provides a mock function with given fields:
func (_m *HealthCheckManager) StartHealthChecks() {
	_m.Called()
}

// HealthCheckManager_StartHealthChecks_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'StartHealthChecks'
type HealthCheckManager_StartHealthChecks_Call struct {
	*mock.Call
}

// StartHealthChecks is a helper method to define mock.On call
func (_e *HealthCheckManager_Expecter) StartHealthChecks() *HealthCheckManager_StartHealthChecks_Call {
	return &HealthCheckManager_StartHealthChecks_Call{Call: _e.mock.On("StartHealthChecks")}
}

func (_c *HealthCheckManager_StartHealthChecks_Call) Run(run func()) *HealthCheckManager_StartHealthChecks_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *HealthCheckManager_StartHealthChecks_Call) Return() *HealthCheckManager_StartHealthChecks_Call {
	_c.Call.Return()
	return _c
}

type mockConstructorTestingTNewHealthCheckManager interface {
	mock.TestingT
	Cleanup(func())
}

// NewHealthCheckManager creates a new instance of HealthCheckManager. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewHealthCheckManager(t mockConstructorTestingTNewHealthCheckManager) *HealthCheckManager {
	mock := &HealthCheckManager{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
