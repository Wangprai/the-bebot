package worker

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"the-bebot/internal/memory/core"
)

// CandidateProcessor processes a claimed memory candidate.
type CandidateProcessor interface {
	Process(
		ctx context.Context,
		candidate core.MemoryCandidate,
	) error
}

// MemoryWorker processes memory candidates in the background.
type MemoryWorker struct {
	repository   core.CandidateRepository
	processor    CandidateProcessor
	workerCount  int
	pollInterval time.Duration
	lockDuration time.Duration
	logger       *slog.Logger

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
}

func NewMemoryWorker(
	repository core.CandidateRepository,
	processor CandidateProcessor,
	workerCount int,
	pollInterval time.Duration,
	lockDuration time.Duration,
	logger *slog.Logger,
) (*MemoryWorker, error) {
	if repository == nil {
		return nil, fmt.Errorf("candidate repository is required")
	}

	if processor == nil {
		return nil, fmt.Errorf("candidate processor is required")
	}

	if workerCount <= 0 {
		return nil, fmt.Errorf("worker count must be greater than zero")
	}

	if pollInterval <= 0 {
		return nil, fmt.Errorf("poll interval must be greater than zero")
	}

	if lockDuration <= 0 {
		return nil, fmt.Errorf("lock duration must be greater than zero")
	}

	if logger == nil {
		return nil, fmt.Errorf("logger is required")
	}

	return &MemoryWorker{
		repository:   repository,
		processor:    processor,
		workerCount:  workerCount,
		pollInterval: pollInterval,
		lockDuration: lockDuration,
		logger:       logger,
	}, nil
}

func (w *MemoryWorker) Start(ctx context.Context) error {
	if w == nil {
		return fmt.Errorf("memory worker is nil")
	}

	if ctx == nil {
		return fmt.Errorf("context is required")
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return fmt.Errorf("memory worker has already started")
	}

	workerCtx, cancel := context.WithCancel(ctx)

	w.started = true
	w.cancel = cancel
	w.done = make(chan struct{})

	var wg sync.WaitGroup

	for i := 0; i < w.workerCount; i++ {
		workerID := fmt.Sprintf("memory-worker-%d", i+1)

		wg.Add(1)

		go func(id string) {
			defer wg.Done()
			w.run(workerCtx, id)
		}(workerID)
	}

	go func() {
		wg.Wait()
		close(w.done)
	}()

	w.logger.Info(
		"Memory worker started",
		"worker_count", w.workerCount,
	)

	return nil
}

func (w *MemoryWorker) Stop(ctx context.Context) error {
	if w == nil {
		return fmt.Errorf("memory worker is nil")
	}

	if ctx == nil {
		return fmt.Errorf("context is required")
	}

	w.mu.Lock()

	if !w.started {
		w.mu.Unlock()
		return nil
	}

	cancel := w.cancel
	done := w.done

	w.mu.Unlock()

	cancel()

	select {
	case <-done:
		w.logger.Info("Memory worker stopped")
		return nil

	case <-ctx.Done():
		return fmt.Errorf(
			"stop memory worker: %w",
			ctx.Err(),
		)
	}
}

func (w *MemoryWorker) run(
	ctx context.Context,
	workerID string,
) {
	w.logger.Info(
		"Memory worker running",
		"worker_id", workerID,
	)

	for {
		if ctx.Err() != nil {
			w.logger.Info(
				"Memory worker shutting down",
				"worker_id", workerID,
			)
			return
		}

		now := time.Now().UTC()

		candidate, err := w.repository.ClaimNext(
			ctx,
			workerID,
			now,
			w.lockDuration,
		)

		if err != nil {
			if ctx.Err() != nil {
				return
			}

			w.logger.Error(
				"Failed to claim memory candidate",
				"worker_id", workerID,
				"error", err,
			)

			if !w.wait(ctx, w.pollInterval) {
				return
			}

			continue
		}

		if candidate == nil {
			if !w.wait(ctx, w.pollInterval) {
				return
			}

			continue
		}

		if err := w.processor.Process(ctx, *candidate); err != nil {
			w.logger.Error(
				"Failed to process memory candidate",
				"worker_id", workerID,
				"candidate_id", candidate.ID,
				"error", err,
			)
		}
	}
}

func (w *MemoryWorker) wait(
	ctx context.Context,
	duration time.Duration,
) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
