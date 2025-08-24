// File: internal/infra/worker/pool.go
package worker

import (
	"context"
	"errors"
	"log"
	"runtime"
	"sync"
)

// A very small worker pool that can run submitted tasks.
// This replaces older types (UserSubscriptionDTO, SubscriptionExecutor).

type Task func(ctx context.Context) error

type Pool struct {
	wg   sync.WaitGroup
	jobs chan Task
	quit chan struct{}
	n    int
}

func NewPool(workers int) *Pool {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	return &Pool{jobs: make(chan Task, workers*4), quit: make(chan struct{}), n: workers}
}

func (p *Pool) Start(ctx context.Context) {
	for i := 0; i < p.n; i++ {
		p.wg.Add(1)
		go func(id int) {
			defer p.wg.Done()
			for {
				select {
				case <-ctx.Done():
					return
				case <-p.quit:
					return
				case task := <-p.jobs:
					if task == nil {
						continue
					}
					if err := task(ctx); err != nil {
						log.Printf("worker %d task error: %v", id, err)
					}
				}
			}
		}(i)
	}
}

func (p *Pool) Stop() {
	close(p.quit)
	p.wg.Wait()
}

func (p *Pool) Submit(task Task) error {
	if task == nil {
		return errors.New("nil task")
	}
	select {
	case p.jobs <- task:
		return nil
	default:
		// drop when saturated to avoid back-pressure in v1
		return errors.New("worker queue full")
	}
}
