package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/shopspring/decimal"
	"github.com/vancuverya-dot/gophermart/internal/config"
)

type Worker struct {
	db     *sql.DB
	client *http.Client
}

type accrualResp struct {
	Status  string          `json:"status"`
	Accrual decimal.Decimal `json:"accrual"`
}

func New(db *sql.DB) *Worker {
	return &Worker{db: db, client: &http.Client{Timeout: 10 * time.Second}}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.process(ctx)
		}
	}
}

func (w *Worker) process(ctx context.Context) {
	rows, err := w.db.QueryContext(ctx,
		`SELECT orders_uuid, orders_code FROM public.orders WHERE orders_status IN ('NEW', 'PROCESSING')`)
	if err != nil {
		log.Printf("worker: query error: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var uuid string
		var code int64
		if err := rows.Scan(&uuid, &code); err != nil {
			continue
		}
		w.processOne(ctx, uuid, code)
	}

	if err := rows.Err(); err != nil { // ← добавь
		log.Printf("worker: rows error: %v", err)
	}
}

func (w *Worker) processOne(ctx context.Context, uuid string, code int64) {
	url := fmt.Sprintf("%s/api/orders/%d", config.AccrualSystemAddress, code)
	resp, err := w.client.Get(url)
	if err != nil {
		log.Printf("worker: get error: %v", err)
		return
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		sec, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		if sec == 0 {
			sec = 60
		}
		time.Sleep(time.Duration(sec) * time.Second)
		return
	case http.StatusOK:
	default:
		return
	}

	var a accrualResp
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return
	}

	w.update(ctx, uuid, a)
}

func (w *Worker) update(ctx context.Context, uuid string, a accrualResp) {
	status := map[string]string{
		"REGISTERED": "NEW",
		"PROCESSING": "PROCESSING",
		"PROCESSED":  "PROCESSED",
		"INVALID":    "INVALID",
	}[a.Status]
	if status == "" {
		status = "NEW"
	}

	tx, err := w.db.BeginTx(ctx, nil)
	if err != nil {
		return
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx,
		`UPDATE public.orders SET orders_status=$1, orders_accrual=$2 WHERE orders_uuid=$3`,
		status, a.Accrual, uuid,
	)
	if err != nil {
		log.Printf("worker: update order error: %v", err)
		return
	}

	if a.Status == "PROCESSED" && !a.Accrual.IsZero() {
		_, err = tx.ExecContext(ctx,
			`UPDATE public.account a SET account_balance = account_balance + $1
			 FROM public.orders o
			 WHERE o.orders_uuid = $2 AND a.account_uuid = o.orders_account_uuid`,
			a.Accrual, uuid,
		)
		if err != nil {
			log.Printf("worker: update balance error: %v", err)
			return
		}
	}

	tx.Commit()
}
