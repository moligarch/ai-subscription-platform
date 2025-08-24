package domain

import "errors"

var (
	// Common domain errors
	ErrNotFound             = errors.New("entity not found")
	ErrNoActiveSubscription = errors.New("no active subscription")
	ErrActiveChatExists     = errors.New("user already has an active chat session")
	ErrAlreadyExists        = errors.New("entity already exists")
	ErrInvalidArgument      = errors.New("invalid argument")
	ErrInsufficientCredits  = errors.New("insufficient credits")
	ErrExpiredSubscription  = errors.New("subscription has expired")
	ErrCodeAlreadyUsed      = errors.New("activation code already used")
	ErrCodeNotFound         = errors.New("activation code not found")
	ErrAlreadyHasReserved   = errors.New("user already has a reserved subscription")
)
