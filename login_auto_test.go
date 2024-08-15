package slackauth

import (
	"errors"
	"testing"

	gomock "go.uber.org/mock/gomock"
)

func Test_enterCode(t *testing.T) {
	type args struct {
		// page pager // provided by test
		code int
	}
	tests := []struct {
		name    string
		args    args
		expect  func(*Mockelementer, *Mockinputter)
		wantErr bool
	}{
		{
			name: "happy path",
			args: args{
				code: 23456,
			},
			expect: func(p *Mockelementer, e *Mockinputter) {
				p.EXPECT().Element(gomock.Any()).Return(e, nil).Times(6)
				e.EXPECT().Input("0").Return(nil).Times(1)
				e.EXPECT().Input("2").Return(nil).Times(1)
				e.EXPECT().Input("3").Return(nil).Times(1)
				e.EXPECT().Input("4").Return(nil).Times(1)
				e.EXPECT().Input("5").Return(nil).Times(1)
				e.EXPECT().Input("6").Return(nil).Times(1)
			},
			wantErr: false,
		},
		{
			name: "invalid code (negative)",
			args: args{
				code: -1,
			},
			expect:  func(*Mockelementer, *Mockinputter) {},
			wantErr: true,
		},
		{
			name: "invalid code (overflow)",
			args: args{
				code: 1000000,
			},
			expect:  func(*Mockelementer, *Mockinputter) {},
			wantErr: true,
		},
		{
			name: "element error",
			args: args{
				code: 23456,
			},
			expect: func(e *Mockelementer, _ *Mockinputter) {
				e.EXPECT().Element(gomock.Any()).Return(nil, errors.New("element error")).Times(1)
			},
			wantErr: true,
		},
		{
			name: "input error",
			args: args{
				code: 23456,
			},
			expect: func(e *Mockelementer, ip *Mockinputter) {
				e.EXPECT().Element(gomock.Any()).Return(ip, nil).Times(1)
				ip.EXPECT().Input(gomock.Any()).Return(errors.New("input error")).Times(1)
			},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			e := NewMockelementer(ctrl)
			ip := NewMockinputter(ctrl)
			tt.expect(e, ip)
			if err := enterCode(e, tt.args.code); (err != nil) != tt.wantErr {
				t.Errorf("enterCode() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
