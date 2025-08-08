package worker

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"telegram-ai-subscription/internal/usecase"
)

// SubscribeTask is a job submitted to the worker pool.
// Caller creates a task and reads result from ResultCh.
type SubscribeTask struct {
	UserID string
	PlanID string
	// ResultCh receives the created subscription or an error.
	ResultCh chan SubscribeResult
	// Optional context for timeout/cancellation of this job
	Ctx context.Context
}

// SubscribeResult is the result of processing a SubscribeTask.
type SubscribeResult struct {
	Subscription *usecase.UserSubscriptionDTO // small DTO to avoid importing domain deeply here
	Err          error
}

// WorkerPool processes SubscribeTask concurrently using a fixed number of workers.
type WorkerPool struct {
	workers int
	tasks   chan *SubscribeTask

	// internal
	wg      sync.WaitGroup
	closed  chan struct{}
	closeMu sync.Mutex

	// dependencies: use case to execute
	subUC *usecase.SubscriptionExecutor
}

// ErrPoolClosed is returned when submitting to a closed pool.
var ErrPoolClosed = errors.New("worker pool closed")

// NewWorkerPool constructs a pool with `workers` goroutines and a buffered task channel of size `queueSize`.
// subUC is the use-case executor that performs the actual Subscribe logic.
func NewWorkerPool(workers, queueSize int, subUC *usecase.SubscriptionExecutor) *WorkerPool {
	if workers <= 0 {
		workers = 4
	}
	if queueSize <= 0 {
		queueSize = workers * 4
	}
	p := &WorkerPool{
		workers: workers,
		tasks:   make(chan *SubscribeTask, queueSize),
		closed:  make(chan struct{}),
		subUC:   subUC,
	}
	p.start()
	return p
}

func (p *WorkerPool) start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.workerLoop(i)
	}
}

func (p *WorkerPool) workerLoop(workerID int) {
	defer p.wg.Done()
	log.Printf("[worker-%d] started\n", workerID)
	for {
		select {
		case <-p.closed:
			log.Printf("[worker-%d] shutting down\n", workerID)
			return
		case task, ok := <-p.tasks:
			if !ok {
				log.Printf("[worker-%d] task channel closed\n", workerID)
				return
			}
			// Always respect task.Ctx if provided; otherwise use background with timeout
			ctx := task.Ctx
			if ctx == nil {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
			}

			// Execute Subscribe via the executor (which wraps usecase.Subscribe)
			sub, err := p.subUC.ExecuteSubscribe(ctx, task.UserID, task.PlanID)
			select {
			case task.ResultCh <- SubscribeResult{Subscription: sub, Err: err}:
				// result delivered
			case <-ctx.Done():
				// if caller timed out or cancelled, we can't deliver result; just continue
				log.Printf("[worker-%d] result delivery cancelled for user=%s\n", workerID, task.UserID)
			}
		}
	}
}

// Submit enqueues a SubscribeTask for processing. Caller provides result channel.
func (p *WorkerPool) Submit(task *SubscribeTask) error {
	p.closeMu.Lock()
	select {
	case <-p.closed:
		p.closeMu.Unlock()
		return ErrPoolClosed
	default:
	}
	p.closeMu.Unlock()

	select {
	case p.tasks <- task:
		return nil
	default:
		// queue full - return error quickly (caller can retry)
		return errors.New("task queue full")
	}
}

// Shutdown gracefully stops accepting tasks and waits for workers to finish.
func (p *WorkerPool) Shutdown(ctx context.Context) error {
	// close tasks channel so workers finish processing queued tasks
	p.closeMu.Lock()
	select {
	case <-p.closed:
		// already closed
		p.closeMu.Unlock()
		return nil
	default:
		close(p.closed) // signal workers to stop
		close(p.tasks)  // close tasks to break workerLoop
	}
	p.closeMu.Unlock()

	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
