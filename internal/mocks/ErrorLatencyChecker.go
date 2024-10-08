// Code generated by mockery v2.43.2. DO NOT EDIT.

package mocks

import (
	types "github.com/satsuma-data/node-gateway/internal/types"
	mock "github.com/stretchr/testify/mock"
)

// ErrorLatencyChecker is an autogenerated mock type for the ErrorLatencyChecker type
type ErrorLatencyChecker struct {
	mock.Mock
}

type ErrorLatencyChecker_Expecter struct {
	mock *mock.Mock
}

func (_m *ErrorLatencyChecker) EXPECT() *ErrorLatencyChecker_Expecter {
	return &ErrorLatencyChecker_Expecter{mock: &_m.Mock}
}

// IsPassing provides a mock function with given fields: methods
func (_m *ErrorLatencyChecker) IsPassing(methods []string) bool {
	ret := _m.Called(methods)

	if len(ret) == 0 {
		panic("no return value specified for IsPassing")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func([]string) bool); ok {
		r0 = rf(methods)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// ErrorLatencyChecker_IsPassing_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'IsPassing'
type ErrorLatencyChecker_IsPassing_Call struct {
	*mock.Call
}

// IsPassing is a helper method to define mock.On call
//   - methods []string
func (_e *ErrorLatencyChecker_Expecter) IsPassing(methods interface{}) *ErrorLatencyChecker_IsPassing_Call {
	return &ErrorLatencyChecker_IsPassing_Call{Call: _e.mock.On("IsPassing", methods)}
}

func (_c *ErrorLatencyChecker_IsPassing_Call) Run(run func(methods []string)) *ErrorLatencyChecker_IsPassing_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].([]string))
	})
	return _c
}

func (_c *ErrorLatencyChecker_IsPassing_Call) Return(_a0 bool) *ErrorLatencyChecker_IsPassing_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *ErrorLatencyChecker_IsPassing_Call) RunAndReturn(run func([]string) bool) *ErrorLatencyChecker_IsPassing_Call {
	_c.Call.Return(run)
	return _c
}

// RecordRequest provides a mock function with given fields: data
func (_m *ErrorLatencyChecker) RecordRequest(data *types.RequestData) bool {
	ret := _m.Called(data)

	if len(ret) == 0 {
		panic("no return value specified for RecordRequest")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func(*types.RequestData) bool); ok {
		r0 = rf(data)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// ErrorLatencyChecker_RecordRequest_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'RecordRequest'
type ErrorLatencyChecker_RecordRequest_Call struct {
	*mock.Call
}

// RecordRequest is a helper method to define mock.On call
//   - data *types.RequestData
func (_e *ErrorLatencyChecker_Expecter) RecordRequest(data interface{}) *ErrorLatencyChecker_RecordRequest_Call {
	return &ErrorLatencyChecker_RecordRequest_Call{Call: _e.mock.On("RecordRequest", data)}
}

func (_c *ErrorLatencyChecker_RecordRequest_Call) Run(run func(data *types.RequestData)) *ErrorLatencyChecker_RecordRequest_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(*types.RequestData))
	})
	return _c
}

func (_c *ErrorLatencyChecker_RecordRequest_Call) Return(_a0 bool) *ErrorLatencyChecker_RecordRequest_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *ErrorLatencyChecker_RecordRequest_Call) RunAndReturn(run func(*types.RequestData) bool) *ErrorLatencyChecker_RecordRequest_Call {
	_c.Call.Return(run)
	return _c
}

// NewErrorLatencyChecker creates a new instance of ErrorLatencyChecker. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewErrorLatencyChecker(t interface {
	mock.TestingT
	Cleanup(func())
}) *ErrorLatencyChecker {
	mock := &ErrorLatencyChecker{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
