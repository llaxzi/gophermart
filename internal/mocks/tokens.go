// Code generated by MockGen. DO NOT EDIT.
// Source: internal/tokens/tokens.go

// Package mocks is a generated GoMock package.
package mocks

import (
	reflect "reflect"

	gomock "github.com/golang/mock/gomock"
)

// MockTokenBuilder is a mock of TokenBuilder interface.
type MockTokenBuilder struct {
	ctrl     *gomock.Controller
	recorder *MockTokenBuilderMockRecorder
}

// MockTokenBuilderMockRecorder is the mock recorder for MockTokenBuilder.
type MockTokenBuilderMockRecorder struct {
	mock *MockTokenBuilder
}

// NewMockTokenBuilder creates a new mock instance.
func NewMockTokenBuilder(ctrl *gomock.Controller) *MockTokenBuilder {
	mock := &MockTokenBuilder{ctrl: ctrl}
	mock.recorder = &MockTokenBuilderMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockTokenBuilder) EXPECT() *MockTokenBuilderMockRecorder {
	return m.recorder
}

// BuildJWTString mocks base method.
func (m *MockTokenBuilder) BuildJWTString(userLogin string) (string, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "BuildJWTString", userLogin)
	ret0, _ := ret[0].(string)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// BuildJWTString indicates an expected call of BuildJWTString.
func (mr *MockTokenBuilderMockRecorder) BuildJWTString(userLogin interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "BuildJWTString", reflect.TypeOf((*MockTokenBuilder)(nil).BuildJWTString), userLogin)
}
