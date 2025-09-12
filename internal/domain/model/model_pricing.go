package model

import (
	"time"

	"github.com/google/uuid"
)

type ModelPricing struct {
	ID                     string
	ModelName              string
	InputTokenPriceMicros  int64
	OutputTokenPriceMicros int64
	Active                 bool
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func NewModelPricing(modelName string, inputPriceMicros, outputPriceMicros int64, active bool) *ModelPricing {
	now := time.Now()
	return &ModelPricing{
		ID:                     uuid.NewString(),
		ModelName:              modelName,
		InputTokenPriceMicros:  inputPriceMicros,
		OutputTokenPriceMicros: outputPriceMicros,
		Active:                 active,
		CreatedAt:              now,
		UpdatedAt:              now,
	}
}
