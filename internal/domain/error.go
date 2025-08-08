package domain

import "errors"

var (
    // Common domain errors
    ErrNotFound            = errors.New("entity not found")
    ErrAlreadyExists       = errors.New("entity already exists")
    ErrInvalidArgument     = errors.New("invalid argument")
    ErrInsufficientCredits = errors.New("insufficient credits")
    ErrExpiredSubscription = errors.New("subscription has expired")
    ErrCodeAlreadyUsed     = errors.New("activation code already used")
    ErrCodeNotFound        = errors.New("activation code not found")
)
