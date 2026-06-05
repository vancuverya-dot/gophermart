package server

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/golang-jwt/jwt/v5"
	"github.com/shopspring/decimal"

	"github.com/vancuverya-dot/gophermart/internal/config"
	"github.com/vancuverya-dot/gophermart/internal/database"
)

func (s *Server) RegisterRoutes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Compress(5))

	r.Post("/api/user/register", s.registerHandler)
	r.Post("/api/user/login", s.loginHandler)

	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware)
		r.Get("/api/user/orders", s.ordersHandler)
		r.Post("/api/user/orders", s.addOrderHandler)
		r.Get("/api/user/balance", s.balanceHandler)
		r.Get("/api/user/withdrawals", s.withdrawalsHandler)
		r.Post("/api/user/balance/withdraw", s.withdrawHandler)
	})

	return r
}

type registerReq struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

// registerHandler — регистрация пользователя. Регистрация производится по паре логин/пароль.
// Каждый логин должен быть уникальным.
// После успешной регистрации должна происходить автоматическая аутентификация пользователя.
// Возможные ответы
// 200 — пользователь успешно зарегистрирован и аутентифицирован;
// 400 — неверный формат запроса;
// 409 — логин уже занят;
// 500 — внутренняя ошибка сервера.
func (s *Server) registerHandler(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if req.Login == "" || req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	uid, err := s.db.Register(r.Context(), req.Login, req.Password)
	if err != nil {
		if errors.Is(err, database.ErrLoginTaken) {
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	token, err := generateToken(uid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Authorization", token)
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		HttpOnly: true,
	})

	w.WriteHeader(http.StatusOK)
}

// loginHandler — аутентификация пользователя. Аутентификация производится по паре логин/пароль.
// Возможные коды ответа:
// 200 — пользователь успешно аутентифицирован;
// 400 — неверный формат запроса;
// 401 — неверная пара логин/пароль;
// 500 — внутренняя ошибка сервера.
func (s *Server) loginHandler(w http.ResponseWriter, r *http.Request) {
	var req registerReq

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if req.Login == "" || req.Password == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	uid, err := s.db.Login(r.Context(), req.Login, req.Password)
	if err != nil {
		if errors.Is(err, database.UserNotFound) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	token, err := generateToken(uid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Authorization", token)
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    token,
		HttpOnly: true,
	})

	w.WriteHeader(http.StatusOK)
}

// AddOrderHandler - загрузка пользователем номера заказа для расчёта.
// Хендлер доступен только аутентифицированным пользователям. Номером заказа является последовательность цифр
// произвольной длины. Номер заказа может быть проверен на корректность ввода с помощью алгоритма Луна.
// Возможные коды ответа:
// 200 — номер заказа уже был загружен этим пользователем;
// 202 — новый номер заказа принят в обработку;
// 400 — неверный формат запроса;
// 401 — пользователь не аутентифицирован;
// 409 — номер заказа уже был загружен другим пользователем;
// 422 — неверный формат номера заказа;
// 500 — внутренняя ошибка сервера.
func (s *Server) addOrderHandler(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	code, err := strconv.ParseInt(strings.TrimSpace(string(body)), 10, 64)
	if err != nil {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	if !luhnCheck(code) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		return
	}

	uid := r.Context().Value("uid").(string)
	err = s.db.InsertOrder(r.Context(), uid, code)
	if err != nil {
		switch {
		case errors.Is(err, database.ErrOrderAlreadyExists):
			w.WriteHeader(http.StatusOK)
		case errors.Is(err, database.ErrOrderTakenByAnother):
			w.WriteHeader(http.StatusConflict)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusAccepted) // 202
	// go func() {
	// 	// TODO: обработка заказа во внешней системе начислений
	// }()
}

// ordersHandler — получение списка загруженных пользователем номеров заказов, статусов их обработки
// и информации о начислениях
// Возможные коды ответа:
// 200 — успешная обработка запроса.
// 204 — нет данных для ответа.
// 401 — пользователь не авторизован.
// 500 — внутренняя ошибка сервера.
func (s *Server) ordersHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value("uid").(string)

	orders, err := s.db.GetOrdersByAccountUUID(r.Context(), uid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(orders)
}

// balanceHandler — Хендлер доступен только авторизованному пользователю. В ответе содержатся данные о текущей сумме
// баллов лояльности, а также сумме использованных за весь период регистрации баллов.
// 200 — успешная обработка запроса.
// 401 — пользователь не авторизован.
// 500 — внутренняя ошибка сервера.
func (s *Server) balanceHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value("uid").(string)

	balance, err := s.db.GetBalance(r.Context(), uid)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(balance)
}

// withdrawHandler — запрос на списание баллов с накопительного счёта в счёт оплаты нового заказа.
// 200 — успешная обработка запроса;
// 401 — пользователь не авторизован;
// 402 — на счету недостаточно средств;
// 422 — неверный номер заказа;
// 500 — внутренняя ошибка сервера.
func (s *Server) withdrawHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Order string          `json:"order"`
		Sum   decimal.Decimal `json:"sum"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	orderNumber, err := strconv.ParseInt(req.Order, 10, 64)
	if err != nil || !luhnCheck(orderNumber) {
		w.WriteHeader(http.StatusUnprocessableEntity) // 422
		return
	}

	err = s.db.Withdraw(r.Context(), r.Context().Value("uid").(string), orderNumber, req.Sum)
	//TODO списание во внешней системе начислений, если заказ не найден статус 422
	if err != nil {
		if errors.Is(err, database.ErrInsufficientFunds) {
			w.WriteHeader(http.StatusPaymentRequired)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// withdrawalsHandler — получение информации о выводе средств с накопительного счёта пользователем.
// Возможные коды ответа:
// 200 — успешная обработка запроса.
// 204 — нет ни одного списания.
// 401 — пользователь не авторизован.
// 500 — внутренняя ошибка сервера.
func (s *Server) withdrawalsHandler(w http.ResponseWriter, r *http.Request) {
	uid := r.Context().Value("uid").(string)

	withdrawals, err := s.db.GetWithdrawals(r.Context(), uid)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(withdrawals)
}

func luhnCheck(number int64) bool {
	if number < 0 {
		return false
	}

	var sum int64
	var alternate bool

	for number > 0 {
		digit := number % 10

		if alternate {
			digit *= 2
			if digit > 9 {
				digit -= 9
			}
		}

		sum += digit
		alternate = !alternate
		number /= 10
	}

	return sum%10 == 0
}

func generateToken(u4 string) (string, error) {
	log.Printf("generating token for uid=%s key=%s", u4, config.TokenKey)
	claims := &Claims{
		UserID:           u4,
		RegisteredClaims: jwt.RegisteredClaims{},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.TokenKey))
}
