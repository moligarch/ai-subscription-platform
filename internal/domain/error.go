package domain

import (
	"errors"
)

var (
	// Common domain errors
	ErrNotFound        = errors.New("entity not found")
	ErrAlreadyExists   = errors.New("entity already exists")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrOperationFailed = errors.New("operation failed")
	ErrRequestFailed   = errors.New("request failed")

	ErrCodeAlreadyUsed = errors.New("activation code already used")
	ErrCodeNotFound    = errors.New("activation code not found")

	ErrEncryptionFailed = errors.New("failed to encrypt content")
	ErrDecryptionFailed = errors.New("failed to decrypt content")
)

// Chat related error
var (
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrModelPricingMissing = errors.New("model pricing missing")
	ErrActiveChatExists    = errors.New("already has an active chat session")
	ErrNoActiveChat        = errors.New("no active session found")
	ErrInitiateChat        = errors.New("failed to initiate chat")
)

// Subscription related error
var (
	ErrNoActiveSubscription      = errors.New("no active subscription")
	ErrExpiredSubscription       = errors.New("subscription has expired")
	ErrAlreadyHasReserved        = errors.New("user already has a reserved subscription")
	ErrSubsciptionWithActiveUser = errors.New("cannot delete plan with active/reserved subscriptions")
)

var (
	ErrInvalidExecContext = errors.New("invalid execution context type: must be pgx.Tx, *pgxpool.Conn, *pgxpool.Pool, or nil")
	ErrReadDatabaseRow    = errors.New("failed to read record from database")
)
