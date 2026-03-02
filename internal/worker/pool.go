package worker

import (
	"context"
	"log"
	"sync"
	"time"

	"uptime-lite/internal/models"
)

// targetRepo is the subset of repository.Repository that Pool requires.
type targetRepo interface {
	ListActiveTargets(ctx context.Context) ([]models.Target, error)
	SaveResult(ctx context.Context, res models.CheckResult) error
}

// statusSetter is the subset of cache.Client that Pool requires.
type statusSetter interface {
	SetStatus(ctx context.Context, s models.TargetStatus) error
}

// Pool manages a fixed number of concurrent HTTP-check workers and a scheduler
// that dispatches targets at their configured intervals.
type Pool struct {
	workers int
	repo    targetRepo
	cache   statusSetter

	jobs chan models.Target
	wg   sync.WaitGroup
}

// NewPool creates a Pool. Call Start to begin processing.
func NewPool(workers int, repo targetRepo, cache statusSetter) *Pool {
	return &Pool{
		workers: workers,
		repo:    repo,
		cache:   cache,
		// Buffer avoids blocking the scheduler when workers are busy.
		jobs: make(chan models.Target, workers*2),
	}
}

// Start launches all worker goroutines and the scheduler. It returns immediately;
// use Wait to block until all goroutines have stopped.
func (p *Pool) Start(ctx context.Context) {
	for i := range p.workers {
		p.wg.Add(1)
		go p.runWorker(ctx, i)
	}
	p.wg.Add(1)
	go p.runScheduler(ctx)
}

// Wait blocks until all workers and the scheduler have finished.
func (p *Pool) Wait() {
	p.wg.Wait()
}

// runWorker reads jobs from the channel and executes HTTP checks until ctx is done.
func (p *Pool) runWorker(ctx context.Context, id int) {
	defer p.wg.Done()
	log.Printf("[worker %d] started", id)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[worker %d] stopped", id)
			return
		case target := <-p.jobs:
			result := check(ctx, target)
			p.persist(ctx, result)
		}
	}
}

// persist writes a check result to PostgreSQL and caches the latest status in Redis.
func (p *Pool) persist(ctx context.Context, result models.CheckResult) {
	if err := p.repo.SaveResult(ctx, result); err != nil {
		log.Printf("[persist] save result for target %d: %v", result.TargetID, err)
	}

	status := models.TargetStatus{
		TargetID:   result.TargetID,
		Up:         result.Up,
		StatusCode: result.StatusCode,
		LatencyMs:  result.LatencyMs,
		CheckedAt:  result.CheckedAt,
	}
	if err := p.cache.SetStatus(ctx, status); err != nil {
		log.Printf("[persist] cache status for target %d: %v", result.TargetID, err)
	}
}

// runScheduler periodically fetches active targets and dispatches those whose
// check interval has elapsed into the jobs channel.
func (p *Pool) runScheduler(ctx context.Context) {
	defer p.wg.Done()

	// lastChecked maps target ID → timestamp of the last dispatched check.
	lastChecked := make(map[int64]time.Time)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Run one dispatch cycle immediately so monitoring starts without a 5s delay.
	p.dispatch(ctx, lastChecked)

	for {
		select {
		case <-ctx.Done():
			log.Println("[scheduler] stopped")
			return
		case <-ticker.C:
			p.dispatch(ctx, lastChecked)
		}
	}
}

// dispatch iterates over active targets and enqueues those that are due for a check.
func (p *Pool) dispatch(ctx context.Context, lastChecked map[int64]time.Time) {
	targets, err := p.repo.ListActiveTargets(ctx)
	if err != nil {
		log.Printf("[scheduler] list active targets: %v", err)
		return
	}

	now := time.Now()
	for _, t := range targets {
		interval := time.Duration(t.IntervalSeconds) * time.Second
		if now.Sub(lastChecked[t.ID]) < interval {
			continue
		}

		select {
		case p.jobs <- t:
			lastChecked[t.ID] = now
		default:
			// Workers are busy; skip this cycle and retry next tick.
			log.Printf("[scheduler] jobs channel full, skipping target %d (%s)", t.ID, t.URL)
		}
	}
}
