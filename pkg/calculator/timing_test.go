package calculator

import (
	"testing"
)

func TestPace(t *testing.T) {
	type args struct {
		distance int
		time     int
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "5000",
			args: args{distance: 5000, time: 1245},
			want: 249,
		},
		{
			name: "10000",
			args: args{distance: 10000, time: 2650},
			want: 265,
		},
		{
			name: "21097",
			args: args{distance: 21097, time: 5928},
			want: 281,
		},
		{
			name: "42195",
			args: args{distance: 42195, time: 13460},
			want: 319,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Pace(tt.args.distance, tt.args.time); got != tt.want {
				t.Errorf("Pace() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTime(t *testing.T) {
	type args struct {
		distance int
		pace     int
	}
	tests := []struct {
		name string
		args args
		want int
	}{
		{
			name: "5000",
			args: args{distance: 5000, pace: 249},
			want: 1245,
		},
		{
			name: "10000",
			args: args{distance: 10000, pace: 265},
			want: 2650,
		},
		{
			name: "21097",
			args: args{distance: 21097, pace: 281},
			want: 5928,
		},
		{
			name: "42195",
			args: args{distance: 42195, pace: 319},
			want: 13460,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Time(tt.args.distance, tt.args.pace); got != tt.want {
				t.Errorf("Time() = %v, want %v", got, tt.want)
			}
		})
	}
}
