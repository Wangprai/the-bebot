package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"the-bebot/internal/memory/core"
)

// ---------- Mocks ----------

type mockWorkerCandidateRepository struct {
	mu sync.Mutex

	candidates []*core.MemoryCandidate
	workerIDs  []string

	claimErr  error
	claimCall int

	deleteErr      error
	deleteCalls    int
	deleteCalledCh chan struct{}
}

func (m *mockWorkerCandidateRepository) Create(
	ctx context.Context,
	candidate core.MemoryCandidate,
) error {
	return nil
}

func (m *mockWorkerCandidateRepository) ClaimNext(
	ctx context.Context,
	workerID string,
	now time.Time,
	lockDuration time.Duration,
) (*core.MemoryCandidate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.claimCall++
	m.workerIDs = append(m.workerIDs, workerID)

	if m.claimErr != nil {
		return nil, m.claimErr
	}

	if len(m.candidates) == 0 {
		return nil, nil
	}

	candidate := m.candidates[0]
	m.candidates = m.candidates[1:]

	return candidate, nil
}

func (m *mockWorkerCandidateRepository) Update(
	ctx context.Context,
	candidate core.MemoryCandidate,
) error {
	return nil
}

func (m *mockWorkerCandidateRepository) UpdateProcessingResult(
	ctx context.Context,
	candidate core.MemoryCandidate,
	expectedWorkerID string,
) error {
	return nil
}

func (m *mockWorkerCandidateRepository) DeleteExpired(
	ctx context.Context,
	before time.Time,
) error {
	m.mu.Lock()
	m.deleteCalls++
	err := m.deleteErr
	ch := m.deleteCalledCh
	m.mu.Unlock()

	if ch != nil {
		select {
		case ch <- struct{}{}:
		default:
		}
	}

	return err
}

type mockWorkerProcessor struct {
	mu sync.Mutex

	processed     []core.MemoryCandidate
	processErr    error
	ignoreContext bool

	processStarted chan struct{}
	releaseProcess chan struct{}
}

func (m *mockWorkerProcessor) Process(
	ctx context.Context,
	candidate core.MemoryCandidate,
) error {
	m.mu.Lock()
	m.processed = append(m.processed, candidate)
	m.mu.Unlock()

	if m.processStarted != nil {
		select {
		case m.processStarted <- struct{}{}:
		default:
		}
	}

	if m.releaseProcess != nil {
		if m.ignoreContext {
			<-m.releaseProcess
		} else {
			select {
			case <-m.releaseProcess:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return m.processErr
}

// ---------- Helpers ----------

func newTestMemoryWorker(
	t *testing.T,
	repository core.CandidateRepository,
	processor CandidateProcessor,
	workerCount int,
) *MemoryWorker {
	t.Helper()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	worker, err := NewMemoryWorker(
		repository,
		processor,
		workerCount,
		10*time.Millisecond,
		time.Minute,
		logger,
	)
	if err != nil {
		t.Fatalf("create memory worker: %v", err)
	}

	return worker
}

func newWorkerTestCandidate(t *testing.T) *core.MemoryCandidate {
	t.Helper()

	now := time.Now().UTC()

	candidate, err := core.NewMemoryCandidate(core.MemoryCandidate{
		ID:        "0123456789abcdef01234567",
		GuildID:   "guild-123",
		ChannelID: "channel-123",
		MessageID: "message-123",
		UserID:    "user-123",
		Content:   "I like Go programming",
		Messages: []core.ConversationMessage{
			{
				MessageID: "message-123",
				UserID:    "user-123",
				Content:   "I like Go programming",
			},
		},
		Status:    core.CandidateStatusPending,
		Attempts:  0,
		ExpireAt:  now.Add(24 * time.Hour),
		CreatedAt: now,
		UpdatedAt: now,
	})
	if err != nil {
		t.Fatalf("create test candidate: %v", err)
	}

	return &candidate
}

// ---------- Tests ----------

func TestNewMemoryWorkerValidation(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	repository := &mockWorkerCandidateRepository{}
	processor := &mockWorkerProcessor{}

	tests := []struct {
		name         string
		repository   core.CandidateRepository
		processor    CandidateProcessor
		workerCount  int
		pollInterval time.Duration
		lockDuration time.Duration
		logger       *slog.Logger
	}{
		{
			name:         "nil repository",
			processor:    processor,
			workerCount:  1,
			pollInterval: time.Second,
			lockDuration: time.Minute,
			logger:       logger,
		},
		{
			name:         "nil processor",
			repository:   repository,
			workerCount:  1,
			pollInterval: time.Second,
			lockDuration: time.Minute,
			logger:       logger,
		},
		{
			name:         "invalid worker count",
			repository:   repository,
			processor:    processor,
			workerCount:  0,
			pollInterval: time.Second,
			lockDuration: time.Minute,
			logger:       logger,
		},
		{
			name:         "invalid poll interval",
			repository:   repository,
			processor:    processor,
			workerCount:  1,
			pollInterval: 0,
			lockDuration: time.Minute,
			logger:       logger,
		},
		{
			name:         "invalid lock duration",
			repository:   repository,
			processor:    processor,
			workerCount:  1,
			pollInterval: time.Second,
			lockDuration: 0,
			logger:       logger,
		},
		{
			name:         "nil logger",
			repository:   repository,
			processor:    processor,
			workerCount:  1,
			pollInterval: time.Second,
			lockDuration: time.Minute,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker, err := NewMemoryWorker(
				tt.repository,
				tt.processor,
				tt.workerCount,
				tt.pollInterval,
				tt.lockDuration,
				tt.logger,
			)

			if err == nil {
				t.Fatal("expected constructor error")
			}

			if worker != nil {
				t.Fatal("expected nil worker")
			}
		})
	}
}

func TestMemoryWorkerStartTwice(t *testing.T) {
	worker := newTestMemoryWorker(
		t,
		&mockWorkerCandidateRepository{},
		&mockWorkerProcessor{},
		1,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := worker.Start(ctx); err != nil {
		t.Fatalf("start worker: %v", err)
	}

	if err := worker.Start(ctx); err == nil {
		t.Fatal("expected error when starting worker twice")
	}

	if err := worker.Stop(context.Background()); err != nil {
		t.Fatalf("stop worker: %v", err)
	}
}

func TestMemoryWorkerProcessesCandidate(t *testing.T) {
	repository := &mockWorkerCandidateRepository{
		candidates: []*core.MemoryCandidate{
			newWorkerTestCandidate(t),
		},
	}

	processor := &mockWorkerProcessor{
		processStarted: make(chan struct{}, 1),
		releaseProcess: make(chan struct{}),
		ignoreContext:  true,
	}

	worker := newTestMemoryWorker(
		t,
		repository,
		processor,
		1,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := worker.Start(ctx); err != nil {
		t.Fatalf("start worker: %v", err)
	}

	select {
	case <-processor.processStarted:
	case <-time.After(time.Second):
		t.Fatal("worker did not process candidate")
	}

	close(processor.releaseProcess)

	if err := worker.Stop(context.Background()); err != nil {
		t.Fatalf("stop worker: %v", err)
	}

	processor.mu.Lock()
	defer processor.mu.Unlock()

	if len(processor.processed) != 1 {
		t.Fatalf(
			"expected 1 processed candidate, got %d",
			len(processor.processed),
		)
	}
}

func TestMemoryWorkerStopWithoutStart(t *testing.T) {
	worker := newTestMemoryWorker(
		t,
		&mockWorkerCandidateRepository{},
		&mockWorkerProcessor{},
		1,
	)

	if err := worker.Stop(context.Background()); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// ---------- Shutdown and Error Tests ----------

func TestMemoryWorkerStopsWhenContextCanceled(t *testing.T) {
	repository := &mockWorkerCandidateRepository{}
	processor := &mockWorkerProcessor{}

	worker := newTestMemoryWorker(
		t,
		repository,
		processor,
		2,
	)

	ctx, cancel := context.WithCancel(context.Background())

	if err := worker.Start(ctx); err != nil {
		t.Fatalf("start worker: %v", err)
	}

	cancel()

	stopCtx, stopCancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer stopCancel()

	if err := worker.Stop(stopCtx); err != nil {
		t.Fatalf("stop worker after context cancellation: %v", err)
	}
}

func TestMemoryWorkerStopTimeout(t *testing.T) {
	processor := &mockWorkerProcessor{
		processStarted: make(chan struct{}, 1),
		releaseProcess: make(chan struct{}),
		ignoreContext:  true,
	}

	repository := &mockWorkerCandidateRepository{
		candidates: []*core.MemoryCandidate{
			newWorkerTestCandidate(t),
		},
	}

	worker := newTestMemoryWorker(
		t,
		repository,
		processor,
		1,
	)

	if err := worker.Start(context.Background()); err != nil {
		t.Fatalf("start worker: %v", err)
	}

	select {
	case <-processor.processStarted:
	case <-time.After(time.Second):
		t.Fatal("worker did not start processing")
	}

	stopCtx, cancel := context.WithTimeout(
		context.Background(),
		20*time.Millisecond,
	)
	defer cancel()

	err := worker.Stop(stopCtx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf(
			"expected context deadline exceeded, got %v",
			err,
		)
	}

	close(processor.releaseProcess)

	// Wait for the worker goroutine to finish after releasing the processor.
	if err := worker.Stop(context.Background()); err != nil {
		t.Fatalf("wait for worker shutdown: %v", err)
	}
}

func TestMemoryWorkerHandlesClaimError(t *testing.T) {
	repository := &mockWorkerCandidateRepository{
		claimErr: errors.New("database unavailable"),
	}

	worker := newTestMemoryWorker(
		t,
		repository,
		&mockWorkerProcessor{},
		1,
	)

	ctx, cancel := context.WithCancel(context.Background())

	if err := worker.Start(ctx); err != nil {
		t.Fatalf("start worker: %v", err)
	}

	deadline := time.After(time.Second)

	for {
		repository.mu.Lock()
		claimCall := repository.claimCall
		repository.mu.Unlock()

		if claimCall > 0 {
			break
		}

		select {
		case <-deadline:
			cancel()
			t.Fatal("worker did not attempt to claim candidate")
		default:
			time.Sleep(time.Millisecond)
		}
	}

	cancel()

	stopCtx, stopCancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer stopCancel()

	if err := worker.Stop(stopCtx); err != nil {
		t.Fatalf("stop worker: %v", err)
	}
}

func TestMemoryWorkerPeriodicDeleteExpiredCleanup(t *testing.T) {
	repository := &mockWorkerCandidateRepository{
		deleteCalledCh: make(chan struct{}, 2),
	}

	worker := newTestMemoryWorker(
		t,
		repository,
		&mockWorkerProcessor{},
		1,
	)

	if err := worker.SetCleanupInterval(15 * time.Millisecond); err != nil {
		t.Fatalf("SetCleanupInterval() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := worker.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	select {
	case <-repository.deleteCalledCh:
	case <-time.After(time.Second):
		t.Fatal("expected periodic DeleteExpired to be invoked")
	}

	if err := worker.SetCleanupInterval(time.Second); err == nil {
		t.Error("expected error when setting cleanup interval after start")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()

	if err := worker.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	repository.mu.Lock()
	calls := repository.deleteCalls
	repository.mu.Unlock()

	if calls < 1 {
		t.Fatalf("expected at least 1 DeleteExpired call, got %d", calls)
	}
}

func TestMemoryWorkerSetCleanupIntervalRejectsNonPositive(t *testing.T) {
	worker := newTestMemoryWorker(
		t,
		&mockWorkerCandidateRepository{},
		&mockWorkerProcessor{},
		1,
	)

	if err := worker.SetCleanupInterval(0); err == nil {
		t.Fatal("expected error for zero cleanup interval")
	}

	if err := worker.SetCleanupInterval(-time.Second); err == nil {
		t.Fatal("expected error for negative cleanup interval")
	}
}
