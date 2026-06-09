package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/vancuverya-dot/gophermart/internal/config"
	"github.com/vancuverya-dot/gophermart/internal/database"
)

const workerCount = 5

type accrualResp struct {
	Status  string          `json:"status"`
	Accrual decimal.Decimal `json:"accrual"`
}

type order struct {
	uuid string
	code int64
}

type Worker struct {
	db     database.Service
	client *http.Client
	jobs   chan order
	mu     sync.RWMutex
}

func New(db database.Service) *Worker {
	return &Worker{
		db:     db,
		client: &http.Client{Timeout: 10 * time.Second},
		jobs:   make(chan order, 100),
	}
}

func (w *Worker) Run(ctx context.Context) {
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.runWorker(ctx)
		}()
	}

	go w.dispatch(ctx)

	wg.Wait()
	log.Println("worker pool stopped")
}

func (w *Worker) dispatch(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			close(w.jobs)
			return
		case <-ticker.C:
			w.mu.RLock()
			w.loadJobs(ctx)
			w.mu.RUnlock()
		}
	}
}

func (w *Worker) loadJobs(ctx context.Context) {
	orders, err := w.db.GetPendingOrders(ctx)
	if err != nil {
		log.Printf("worker: query error: %v", err)
		return
	}
	for _, o := range orders {
		select {
		case w.jobs <- order{uuid: o.UUID, code: o.Code}:
		case <-ctx.Done():
			return
		default:
			return
		}
	}
}

func (w *Worker) runWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case o, ok := <-w.jobs:
			if !ok {
				return
			}

			w.mu.RLock()
			w.processOne(ctx, o)
			w.mu.RUnlock()
		}
	}
}

func (w *Worker) processOne(ctx context.Context, o order) {
	url := fmt.Sprintf("%s/api/orders/%d", config.AccrualSystemAddress, o.code)

	resp, err := w.client.Get(url)
	if err != nil {
		log.Printf("worker: request error: %v", err)
		return
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		sec, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		if sec == 0 {
			sec = 60
		}
		log.Printf("worker: hit 429, freezing all workers for %d seconds", sec)

		w.mu.RUnlock()
		w.mu.Lock()

		select {
		case <-time.After(time.Duration(sec) * time.Second):
		case <-ctx.Done():
			w.mu.Unlock()
			return
		}

		w.mu.Unlock()
		w.mu.RLock()

		select {
		case w.jobs <- o:
		case <-ctx.Done():
		}

	case http.StatusOK:
		var a accrualResp
		if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
			log.Printf("worker: decode error: %v", err)
			return
		}
		w.update(ctx, o.uuid, a)

	case http.StatusNoContent:
		log.Printf("worker: order %d not found in accrual", o.code)
	}
}

func (w *Worker) update(ctx context.Context, uuid string, a accrualResp) {
	status := map[string]string{
		"REGISTERED": "NEW",
		"PROCESSING": "PROCESSING",
		"PROCESSED":  "PROCESSED",
		"INVALID":    "INVALID",
	}[a.Status]
	if status == "" {
		status = "PROCESSING"
	}

	if err := w.db.UpdateOrderAccrual(ctx, uuid, status, a.Accrual); err != nil {
		log.Printf("worker: update error: %v", err)
	}
}
