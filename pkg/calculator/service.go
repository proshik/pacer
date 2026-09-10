package calculator

import (
	"time"
)

type Service struct{}

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
