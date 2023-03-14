// Code generated by mockery v2.20.0. DO NOT EDIT.

package mocks

import mock "github.com/stretchr/testify/mock"

// BlockHeightChecker is an autogenerated mock type for the BlockHeightChecker type
type BlockHeightChecker struct {
	mock.Mock
}

type BlockHeightChecker_Expecter struct {
	mock *mock.Mock
}

func (_m *BlockHeightChecker) EXPECT() *BlockHeightChecker_Expecter {
	return &BlockHeightChecker_Expecter{mock: &_m.Mock}
}

// GetBlockHeight provides a mock function with given fields:
func (_m *BlockHeightChecker) GetBlockHeight() uint64 {
	ret := _m.Called()

	var r0 uint64
	if rf, ok := ret.Get(0).(func() uint64); ok {
		r0 = rf()
	} else {
		r0 = ret.Get(0).(uint64)
	}

	return r0
}

// BlockHeightChecker_GetBlockHeight_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetBlockHeight'
type BlockHeightChecker_GetBlockHeight_Call struct {
	*mock.Call
}

// GetBlockHeight is a helper method to define mock.On call
func (_e *BlockHeightChecker_Expecter) GetBlockHeight() *BlockHeightChecker_GetBlockHeight_Call {
	return &BlockHeightChecker_GetBlockHeight_Call{Call: _e.mock.On("GetBlockHeight")}
}

func (_c *BlockHeightChecker_GetBlockHeight_Call) Run(run func()) *BlockHeightChecker_GetBlockHeight_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *BlockHeightChecker_GetBlockHeight_Call) Return(_a0 uint64) *BlockHeightChecker_GetBlockHeight_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *BlockHeightChecker_GetBlockHeight_Call) RunAndReturn(run func() uint64) *BlockHeightChecker_GetBlockHeight_Call {
	_c.Call.Return(run)
	return _c
}

// GetError provides a mock function with given fields:
func (_m *BlockHeightChecker) GetError() error {
	ret := _m.Called()

	var r0 error
	if rf, ok := ret.Get(0).(func() error); ok {
		r0 = rf()
	} else {
		r0 = ret.Error(0)
	}

	return r0
}

// BlockHeightChecker_GetError_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'GetError'
type BlockHeightChecker_GetError_Call struct {
	*mock.Call
}

// GetError is a helper method to define mock.On call
func (_e *BlockHeightChecker_Expecter) GetError() *BlockHeightChecker_GetError_Call {
	return &BlockHeightChecker_GetError_Call{Call: _e.mock.On("GetError")}
}

func (_c *BlockHeightChecker_GetError_Call) Run(run func()) *BlockHeightChecker_GetError_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *BlockHeightChecker_GetError_Call) Return(_a0 error) *BlockHeightChecker_GetError_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *BlockHeightChecker_GetError_Call) RunAndReturn(run func() error) *BlockHeightChecker_GetError_Call {
	_c.Call.Return(run)
	return _c
}

// IsPassing provides a mock function with given fields: maxBlockHeight
func (_m *BlockHeightChecker) IsPassing(maxBlockHeight uint64) bool {
	ret := _m.Called(maxBlockHeight)

	var r0 bool
	if rf, ok := ret.Get(0).(func(uint64) bool); ok {
		r0 = rf(maxBlockHeight)
	} else {
		r0 = ret.Get(0).(bool)
	}

	return r0
}

// BlockHeightChecker_IsPassing_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'IsPassing'
type BlockHeightChecker_IsPassing_Call struct {
	*mock.Call
}

// IsPassing is a helper method to define mock.On call
//   - maxBlockHeight uint64
func (_e *BlockHeightChecker_Expecter) IsPassing(maxBlockHeight interface{}) *BlockHeightChecker_IsPassing_Call {
	return &BlockHeightChecker_IsPassing_Call{Call: _e.mock.On("IsPassing", maxBlockHeight)}
}

func (_c *BlockHeightChecker_IsPassing_Call) Run(run func(maxBlockHeight uint64)) *BlockHeightChecker_IsPassing_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run(args[0].(uint64))
	})
	return _c
}

func (_c *BlockHeightChecker_IsPassing_Call) Return(_a0 bool) *BlockHeightChecker_IsPassing_Call {
	_c.Call.Return(_a0)
	return _c
}

func (_c *BlockHeightChecker_IsPassing_Call) RunAndReturn(run func(uint64) bool) *BlockHeightChecker_IsPassing_Call {
	_c.Call.Return(run)
	return _c
}

// RunCheck provides a mock function with given fields:
func (_m *BlockHeightChecker) RunCheck() {
	_m.Called()
}

// BlockHeightChecker_RunCheck_Call is a *mock.Call that shadows Run/Return methods with type explicit version for method 'RunCheck'
type BlockHeightChecker_RunCheck_Call struct {
	*mock.Call
}

// RunCheck is a helper method to define mock.On call
func (_e *BlockHeightChecker_Expecter) RunCheck() *BlockHeightChecker_RunCheck_Call {
	return &BlockHeightChecker_RunCheck_Call{Call: _e.mock.On("RunCheck")}
}

func (_c *BlockHeightChecker_RunCheck_Call) Run(run func()) *BlockHeightChecker_RunCheck_Call {
	_c.Call.Run(func(args mock.Arguments) {
		run()
	})
	return _c
}

func (_c *BlockHeightChecker_RunCheck_Call) Return() *BlockHeightChecker_RunCheck_Call {
	_c.Call.Return()
	return _c
}

func (_c *BlockHeightChecker_RunCheck_Call) RunAndReturn(run func()) *BlockHeightChecker_RunCheck_Call {
	_c.Call.Return(run)
	return _c
}

type mockConstructorTestingTNewBlockHeightChecker interface {
	mock.TestingT
	Cleanup(func())
}

// NewBlockHeightChecker creates a new instance of BlockHeightChecker. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewBlockHeightChecker(t mockConstructorTestingTNewBlockHeightChecker) *BlockHeightChecker {
	mock := &BlockHeightChecker{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
