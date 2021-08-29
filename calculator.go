package main

import "time"

type Unit int

const (
	metric Unit = iota
	imperial
)

type Calculator struct {
	//unit Unit
}

func NewCalculator() *Calculator {
	return &Calculator{}
}

func (c *Calculator) Time(dist int, pace time.Duration) time.Duration {
	resultTime := Time(dist, int(pace.Seconds()))

	return time.Duration(resultTime) * time.Second
}

func (c *Calculator) Pace(dist int, timeValue time.Duration) time.Duration {
	resultPace := Pace(dist, int(timeValue.Seconds()))

	return time.Duration(resultPace) * time.Second
}
