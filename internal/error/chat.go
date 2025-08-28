package derror

import "errors"

var (
	ErrInsufficientCredits = errors.New("insufficient credits")
	ErrModelPricingMissing = errors.New("model pricing missing")
	ErrActiveChatExists    = errors.New("already has an active chat session")
	ErrNoActiveChat        = errors.New("no active session found")
)
