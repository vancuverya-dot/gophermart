package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	retryablehttp "github.com/hashicorp/go-retryablehttp"
	"github.com/shopspring/decimal"
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
	db                   database.Service
	client               *http.Client
	jobs                 chan order
	pauseUntil           atomic.Pointer[time.Time] // timestamp до которого воркеры не делают запросы
	accrualSystemAddress string
}

func New(db database.Service, accrualSystemAddress string) *Worker {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.Logger = nil

	return &Worker{
		db:                   db,
		client:               retryClient.StandardClient(),
		jobs:                 make(chan order, 100),
		accrualSystemAddress: accrualSystemAddress,
	}
}

func (w *Worker) Run(ctx context.Context, interval time.Duration) {
	var wg sync.WaitGroup

	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.runWorker(ctx)
		}()
	}

	go w.dispatch(ctx, interval)

	wg.Wait()
	log.Println("worker pool stopped")
}

func (w *Worker) dispatch(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			close(w.jobs)
			return
		case <-ticker.C:
			w.loadJobs(ctx)
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

			if until := w.pauseUntil.Load(); until != nil && time.Now().Before(*until) {
				select {
				case <-ctx.Done():
					return
				case <-time.After(time.Until(*until)):
				}
			}

			w.processOne(ctx, o)
		}
	}
}

func (w *Worker) processOne(ctx context.Context, o order) {
	url := fmt.Sprintf("%s/api/orders/%d", w.accrualSystemAddress, o.code)

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
		log.Printf("worker: hit 429, pausing all workers for %d seconds", sec)

		until := time.Now().Add(time.Duration(sec) * time.Second)
		w.pauseUntil.Store(&until)

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
