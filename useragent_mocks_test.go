// Code generated by MockGen. DO NOT EDIT.
// Source: useragent.go
//
// Generated by this command:
//
//	mockgen -destination=useragent_mocks_test.go -package=slackauth -source=useragent.go userAgentSetter
//

// Package slackauth is a generated GoMock package.
package slackauth

import (
	reflect "reflect"

	proto "github.com/go-rod/rod/lib/proto"
	gomock "go.uber.org/mock/gomock"
)

// MockuserAgentSetter is a mock of userAgentSetter interface.
type MockuserAgentSetter struct {
	ctrl     *gomock.Controller
	recorder *MockuserAgentSetterMockRecorder
}

// MockuserAgentSetterMockRecorder is the mock recorder for MockuserAgentSetter.
type MockuserAgentSetterMockRecorder struct {
	mock *MockuserAgentSetter
}

// NewMockuserAgentSetter creates a new mock instance.
func NewMockuserAgentSetter(ctrl *gomock.Controller) *MockuserAgentSetter {
	mock := &MockuserAgentSetter{ctrl: ctrl}
	mock.recorder = &MockuserAgentSetterMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockuserAgentSetter) EXPECT() *MockuserAgentSetterMockRecorder {
	return m.recorder
}

// SetUserAgent mocks base method.
func (m *MockuserAgentSetter) SetUserAgent(req *proto.NetworkSetUserAgentOverride) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SetUserAgent", req)
	ret0, _ := ret[0].(error)
	return ret0
}

// SetUserAgent indicates an expected call of SetUserAgent.
func (mr *MockuserAgentSetterMockRecorder) SetUserAgent(req any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetUserAgent", reflect.TypeOf((*MockuserAgentSetter)(nil).SetUserAgent), req)
}
