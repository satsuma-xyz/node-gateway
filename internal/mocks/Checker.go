// Code generated by mockery v2.38.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// Checker is an autogenerated mock type for the Checker type
type Checker struct {
	mock.Mock
}

type Checker_Expecter struct {
	mock *mock.Mock
}

func (_m *Checker) EXPECT() *Checker_Expecter {
	return &Checker_Expecter{mock: &_m.Mock}
}

// IsPassing provides a mock function with given fields:
func (_m *Checker) IsPassing() bool {
	ret := _m.Called()

	if len(ret) == 0 {
		panic("no return value specified for IsPassing")
	}

	var r0 bool
	if rf, ok := ret.Get(0).(func() bool); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// Checker_IsPassing_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'IsPassing'
type Checker_IsPassing_Call struct {
	*mock.Call
}

// IsPassing is a helper method to define mock.On call
func (_e *Checker_Expecter) IsPassing() *Checker_IsPassing_Call {
	return &Checker_IsPassing_Call{Call: _e.mock.On("IsPassing")}
}

func (_c *Checker_IsPassing_Call) Run(run func()) *Checker_IsPassing_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Checker_IsPassing_Call) Return(_a0 bool) *Checker_IsPassing_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *Checker_IsPassing_Call) RunAndReturn(run func() bool) *Checker_IsPassing_Call {
	_c.Call.Return(run)
	return _c
}

// RunCheck provides a mock function with given fields:
func (_m *Checker) RunCheck() {
	_m.Called()
}

// Checker_RunCheck_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'RunCheck'
type Checker_RunCheck_Call struct {
	*mock.Call
}

// RunCheck is a helper method to define mock.On call
func (_e *Checker_Expecter) RunCheck() *Checker_RunCheck_Call {
	return &Checker_RunCheck_Call{Call: _e.mock.On("RunCheck")}
}

func (_c *Checker_RunCheck_Call) Run(run func()) *Checker_RunCheck_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *Checker_RunCheck_Call) Return() *Checker_RunCheck_Call {
	_c.Call.Return()
	return _c
}

func (_c *Checker_RunCheck_Call) RunAndReturn(run func()) *Checker_RunCheck_Call {
	_c.Call.Return(run)
	return _c
}

// NewChecker creates a new instance of Checker. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
// The first argument is typically a *testing.T value.
func NewChecker(t interface {
	mock.TestingT
	Cleanup(func())
}) *Checker {
	mock := &Checker{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
