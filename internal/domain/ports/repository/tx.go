package repository

import (
	"context"

	"github.com/jackc/pgx/v4"
)

type Tx interface{}

var NoTX interface{}

// TransactionManager provides a thin abstraction to execute a function within a
// database transaction, passing the underlying transaction handle via `qx`.
//
// RATIONALE
// - Keeps use-case interfaces clean (no transaction types leaking out).
// - Allows repository methods that accept `qx any` to detect a tx (implementation-side)
// and run SELECT ... FOR UPDATE / use tx-bound Exec/Query as needed.
// - Works across different storage backends as long as they can provide a tx handle.
//
// USAGE
// tm.WithTx(ctx, func(ctx context.Context, qx any) error {
// // call repositories with the same ctx and qx
// p, err := payments.FindByID(ctx, qx, id)
// ...
// return err
// })
//
// The concrete type of `qx` is infra-defined (e.g., pgx.Tx for Postgres).
// Repositories MUST gracefully accept `nil` qx (non-transactional path).
//
// Keep this interface small and stable.
type TransactionManager interface {
	WithTx(ctx context.Context, txOpt pgx.TxOptions, fn func(ctx context.Context, tx Tx) error) error
}
