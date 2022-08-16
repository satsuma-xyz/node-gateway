// Code generated by mockery v2.14.0. DO NOT EDIT.

package mocks

import (
	http "net/http"

	jsonrpc "github.com/satsuma-data/node-gateway/internal/jsonrpc"

	mock "github.com/stretchr/testify/mock"
)

// Router is an autogenerated mock type for the Router type
type Router struct {
	mock.Mock
}

// Route provides a mock function with given fields: requestBody
func (_m *Router) Route(requestBody jsonrpc.RequestBody) (jsonrpc.ResponseBody, *http.Response, error) {
	ret := _m.Called(requestBody)

	var r0 jsonrpc.ResponseBody
	if rf, ok := ret.Get(0).(func(jsonrpc.RequestBody) jsonrpc.ResponseBody); ok {
		r0 = rf(requestBody)
	} else {
		r0 = ret.Get(0).(jsonrpc.ResponseBody)
	}

	var r1 *http.Response
	if rf, ok := ret.Get(1).(func(jsonrpc.RequestBody) *http.Response); ok {
		r1 = rf(requestBody)
	} else {
		if ret.Get(1) != nil {
			r1 = ret.Get(1).(*http.Response)
		}
	}

	var r2 error
	if rf, ok := ret.Get(2).(func(jsonrpc.RequestBody) error); ok {
		r2 = rf(requestBody)
	} else {
		r2 = ret.Error(2)
	}

	return r0, r1, r2
}

type mockConstructorTestingTNewRouter interface {
	mock.TestingT
	Cleanup(func())
}

// NewRouter creates a new instance of Router. It also registers a testing interface on the mock and a cleanup function to assert the mocks expectations.
func NewRouter(t mockConstructorTestingTNewRouter) *Router {
	mock := &Router{}
	mock.Mock.Test(t)

	t.Cleanup(func() { mock.AssertExpectations(t) })

	return mock
}
