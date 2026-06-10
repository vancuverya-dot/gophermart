package server

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vancuverya-dot/gophermart/internal/database"

	"github.com/shopspring/decimal"
)

// mockDB — мок базы данных для тестов
type mockDB struct {
	registerFn           func(ctx context.Context, login, pass string) (string, error)
	loginFn              func(ctx context.Context, login, pass string) (string, error)
	insertOrderFn        func(ctx context.Context, accountUUID string, orderCode int64) error
	getOrdersFn          func(ctx context.Context, accountUUID string) ([]map[string]interface{}, error)
	getBalanceFn         func(ctx context.Context, accountUUID string) (map[string]interface{}, error)
	withdrawFn           func(ctx context.Context, accountUUID string, orderNumber int64, sum decimal.Decimal) error
	getWithdrawalsFn     func(ctx context.Context, accountUUID string) ([]map[string]interface{}, error)
	getPendingOrdersFn   func(ctx context.Context) ([]database.PendingOrder, error)
	updateOrderAccrualFn func(ctx context.Context, uuid string, status string, accrual decimal.Decimal) error
}

func (m *mockDB) Register(ctx context.Context, login, pass string) (string, error) {
	return m.registerFn(ctx, login, pass)
}
func (m *mockDB) Login(ctx context.Context, login, pass string) (string, error) {
	return m.loginFn(ctx, login, pass)
}
func (m *mockDB) InsertOrder(ctx context.Context, accountUUID string, orderCode int64) error {
	return m.insertOrderFn(ctx, accountUUID, orderCode)
}
func (m *mockDB) GetOrdersByAccountUUID(ctx context.Context, accountUUID string) ([]map[string]interface{}, error) {
	return m.getOrdersFn(ctx, accountUUID)
}
func (m *mockDB) GetBalance(ctx context.Context, accountUUID string) (map[string]interface{}, error) {
	return m.getBalanceFn(ctx, accountUUID)
}
func (m *mockDB) Withdraw(ctx context.Context, accountUUID string, orderNumber int64, sum decimal.Decimal) error {
	return m.withdrawFn(ctx, accountUUID, orderNumber, sum)
}
func (m *mockDB) GetWithdrawals(ctx context.Context, accountUUID string) ([]map[string]interface{}, error) {
	return m.getWithdrawalsFn(ctx, accountUUID)
}
func (m *mockDB) GetPendingOrders(ctx context.Context) ([]database.PendingOrder, error) {
	if m.getPendingOrdersFn == nil {
		return nil, nil
	}
	return m.getPendingOrdersFn(ctx)
}
func (m *mockDB) UpdateOrderAccrual(ctx context.Context, uuid string, status string, accrual decimal.Decimal) error {
	if m.updateOrderAccrualFn == nil {
		return nil
	}
	return m.updateOrderAccrualFn(ctx, uuid, status, accrual)
}
func (m *mockDB) Close() error { return nil }

func (m *mockDB) DB() *sql.DB { return nil }

// newTestServer — создаёт тестовый сервер с мок БД
func newTestServer(db *mockDB) *Server {
	return &Server{db: db}
}

// TestRegisterHandler — тест регистрации пользователя
func TestRegisterHandler(t *testing.T) {
	tests := []struct {
		name       string
		body       map[string]string
		mockFn     func(ctx context.Context, login, pass string) (string, error)
		wantStatus int
	}{
		{
			name:       "success",
			body:       map[string]string{"login": "user1", "password": "pass1"},
			mockFn:     func(ctx context.Context, login, pass string) (string, error) { return "uuid-1", nil },
			wantStatus: http.StatusOK,
		},
		{
			name:       "login taken",
			body:       map[string]string{"login": "user1", "password": "pass1"},
			mockFn:     func(ctx context.Context, login, pass string) (string, error) { return "", database.ErrLoginTaken },
			wantStatus: http.StatusConflict,
		},
		{
			name:       "empty login",
			body:       map[string]string{"login": "", "password": "pass1"},
			mockFn:     func(ctx context.Context, login, pass string) (string, error) { return "", nil },
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "empty password",
			body:       map[string]string{"login": "user1", "password": ""},
			mockFn:     func(ctx context.Context, login, pass string) (string, error) { return "", nil },
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(&mockDB{registerFn: tt.mockFn})

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/user/register", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			s.registerHandler(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

// TestLoginHandler — тест аутентификации пользователя
func TestLoginHandler(t *testing.T) {
	tests := []struct {
		name       string
		body       map[string]string
		mockFn     func(ctx context.Context, login, pass string) (string, error)
		wantStatus int
	}{
		{
			name:       "success",
			body:       map[string]string{"login": "user1", "password": "pass1"},
			mockFn:     func(ctx context.Context, login, pass string) (string, error) { return "uuid-1", nil },
			wantStatus: http.StatusOK,
		},
		{
			name:       "user not found",
			body:       map[string]string{"login": "user1", "password": "wrong"},
			mockFn:     func(ctx context.Context, login, pass string) (string, error) { return "", database.ErrUserNotFound },
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "empty fields",
			body:       map[string]string{"login": "", "password": ""},
			mockFn:     func(ctx context.Context, login, pass string) (string, error) { return "", nil },
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(&mockDB{loginFn: tt.mockFn})

			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest(http.MethodPost, "/api/user/login", bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			s.loginHandler(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

// TestAddOrderHandler — тест загрузки номера заказа
func TestAddOrderHandler(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		mockFn     func(ctx context.Context, accountUUID string, orderCode int64) error
		wantStatus int
	}{
		{
			name:       "success",
			body:       "12345678903",
			mockFn:     func(ctx context.Context, accountUUID string, orderCode int64) error { return nil },
			wantStatus: http.StatusAccepted,
		},
		{
			name: "order already exists",
			body: "12345678903",
			mockFn: func(ctx context.Context, accountUUID string, orderCode int64) error {
				return database.ErrOrderAlreadyExists
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "order taken by another",
			body: "12345678903",
			mockFn: func(ctx context.Context, accountUUID string, orderCode int64) error {
				return database.ErrOrderTakenByAnother
			},
			wantStatus: http.StatusConflict,
		},
		{
			name:       "invalid order number",
			body:       "123",
			mockFn:     func(ctx context.Context, accountUUID string, orderCode int64) error { return nil },
			wantStatus: http.StatusUnprocessableEntity,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(&mockDB{insertOrderFn: tt.mockFn})

			req := httptest.NewRequest(http.MethodPost, "/api/user/orders", bytes.NewBufferString(tt.body))
			req = req.WithContext(context.WithValue(req.Context(), "uid", "uuid-1"))
			w := httptest.NewRecorder()

			s.addOrderHandler(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

// TestBalanceHandler — тест получения баланса
func TestBalanceHandler(t *testing.T) {
	tests := []struct {
		name       string
		mockFn     func(ctx context.Context, accountUUID string) (map[string]interface{}, error)
		wantStatus int
	}{
		{
			name: "success",
			mockFn: func(ctx context.Context, accountUUID string) (map[string]interface{}, error) {
				return map[string]interface{}{"current": decimal.NewFromInt(100), "withdrawn": decimal.Zero}, nil
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "account not found",
			mockFn: func(ctx context.Context, accountUUID string) (map[string]interface{}, error) {
				return nil, database.ErrAccountNotFound
			},
			wantStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(&mockDB{getBalanceFn: tt.mockFn})

			req := httptest.NewRequest(http.MethodGet, "/api/user/balance", nil)
			req = req.WithContext(context.WithValue(req.Context(), "uid", "uuid-1"))
			w := httptest.NewRecorder()

			s.balanceHandler(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("got %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestLuhnCheck(t *testing.T) {
	tests := []struct {
		name     string
		number   int64
		expected bool
	}{
		{"valid number", 4532015112830366, true},
		{"valid number from tz", 12345678903, true},
		{"invalid number", 1234567890, false},
		{"negative number", -1234567890, false},
		{"single digit", 5, false},
		{"zero", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := luhnCheck(tt.number)
			if result != tt.expected {
				t.Errorf("luhnCheck(%d) = %v, want %v", tt.number, result, tt.expected)
			}
		})
	}
}
