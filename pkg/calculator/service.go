package calculator

import (
	"time"
)

// Engine computes running pace and finish time. Both the natively linked
// Service and the wasm backed engine in pkg/wasmcalc implement it, so a host
// can swap one for the other without touching call sites.
type Engine interface {
	Time(dist int, pace time.Duration) time.Duration
	Pace(dist int, timeValue time.Duration) time.Duration
}

type Service struct{}

var _ Engine = (*Service)(nil)

func NewService() *Service {
	return &Service{}
}

func (s *Service) Time(dist int, pace time.Duration) time.Duration {
	resultTime := Time(dist, int(pace.Seconds()))

	return time.Duration(resultTime) * time.Second
}

func (s *Service) Pace(dist int, timeValue time.Duration) time.Duration {
	resultPace := Pace(dist, int(timeValue.Seconds()))

	return time.Duration(resultPace) * time.Second
}
