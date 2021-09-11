package calculator

import (
	"time"
)

type Unit int

const (
	metric Unit = iota
	imperial
)

type Service struct {
	//unit Unit
}

func NewService() *Service {
	return &Service{}
}

func (c *Service) Time(dist int, pace time.Duration) time.Duration {
	resultTime := Time(dist, int(pace.Seconds()))

	return time.Duration(resultTime) * time.Second
}

func (c *Service) Pace(dist int, timeValue time.Duration) time.Duration {
	resultPace := Pace(dist, int(timeValue.Seconds()))

	return time.Duration(resultPace) * time.Second
}
