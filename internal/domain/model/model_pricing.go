package model

import "time"

type ModelPricing struct {
	ID                     string
	ModelName              string
	InputTokenPriceMicros  int64
	OutputTokenPriceMicros int64
	Active                 bool
	CreatedAt              time.Time
	UpdatedAt              time.Time
}
