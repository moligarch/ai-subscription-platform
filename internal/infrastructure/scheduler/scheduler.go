package scheduler

import (
	"context"
	"log"
	"time"
)

// Notifier is the minimal interface the scheduler needs from a notification use-case.
// Any type implementing CheckAndNotify(context.Context,int) (int,error) can be passed.
type Notifier interface {
	// CheckAndNotify finds subscriptions expiring within `withinDays` and sends notifications.
	// Returns number of notifications sent and an error (first error) if any.
	CheckAndNotify(ctx context.Context, withinDays int) (int, error)
}

// Scheduler periodically runs a Notifier's CheckAndNotify method.
type Scheduler struct {
	interval time.Duration
	notifier Notifier

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// NewScheduler constructs a scheduler that runs notifier.CheckAndNotify every `interval`.
// If interval <= 0 it defaults to 1 minute.
func NewScheduler(interval time.Duration, notifier Notifier) *Scheduler {
	if interval <= 0 {
		interval = time.Minute
	}
	return &Scheduler{
		interval: interval,
		notifier: notifier,
		done:     make(chan struct{}),
	}
}

// Start begins the scheduler loop in a background goroutine.
// parentCtx is used as the parent for internal contexts; calling Start multiple times has no effect.
func (s *Scheduler) Start(parentCtx context.Context) {
	if s.ctx != nil {
		// already started
		return
	}
	ctx, cancel := context.WithCancel(parentCtx)
	s.ctx = ctx
	s.cancel = cancel

	go s.loop()
}

// loop runs the periodic job until cancelled.
func (s *Scheduler) loop() {
	ticker := time.NewTicker(s.interval)
	defer func() {
		ticker.Stop()
		close(s.done)
	}()

	log.Printf("[scheduler] started with interval %s\n", s.interval)
	for {
		select {
		case <-s.ctx.Done():
			log.Println("[scheduler] context cancelled; stopping")
			return
		case <-ticker.C:
			// run CheckAndNotify with a bounded timeout
			runCtx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
			func() {
				defer cancel()
				sent, err := s.notifier.CheckAndNotify(runCtx, 2)
				if err != nil {
					log.Printf("[scheduler] CheckAndNotify error: %v", err)
					return
				}
				if sent > 0 {
					log.Printf("[scheduler] sent %d notifications", sent)
				}
			}()
		}
	}
}

// Stop cancels the scheduler and waits for the loop to finish. It is idempotent.
func (s *Scheduler) Stop() {
	if s.cancel == nil {
		// not started
		return
	}
	// cancel and wait for done
	s.cancel()
	<-s.done
	// reset for potential restart
	s.ctx = nil
	s.cancel = nil
	s.done = make(chan struct{})
	log.Println("[scheduler] stopped")
}
