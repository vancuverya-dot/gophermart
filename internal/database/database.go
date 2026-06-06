package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/vancuverya-dot/gophermart/internal/config"
	"github.com/vancuverya-dot/gophermart/internal/migrations"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/shopspring/decimal"
)

type Service interface {
	Close() error
	Register(ctx context.Context, login, pass string) (string, error)
	Login(ctx context.Context, login, pass string) (string, error)
	InsertOrder(ctx context.Context, accountUUID string, orderCode int64) error
	GetOrdersByAccountUUID(ctx context.Context, accountUUID string) ([]map[string]interface{}, error)
	GetBalance(ctx context.Context, accountUUID string) (map[string]interface{}, error)
	Withdraw(ctx context.Context, accountUUID string, orderNumber int64, sum decimal.Decimal) error
	GetWithdrawals(ctx context.Context, accountUUID string) ([]map[string]interface{}, error)
	DB() *sql.DB
}

type service struct {
	db *sql.DB
}

var dbInstance *service

func New() Service {
	if dbInstance != nil {
		return dbInstance
	}

	db, err := sql.Open("pgx", config.DBURI)
	if err != nil {
		log.Fatal(err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(10)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := migrations.Up(db); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	dbInstance = &service{
		db: db,
	}
	return dbInstance
}

var ErrLoginTaken = errors.New("login already taken")
var ErrUserNotFound = errors.New("user not found")
var ErrOrderAlreadyExists = errors.New("order already uploaded by this user")
var ErrOrderTakenByAnother = errors.New("order already uploaded by another user")
var ErrAccountNotFound = errors.New("account not found")
var ErrInsufficientFunds = errors.New("insufficient funds")

// registerHandler — регистрация пользователя. Регистрация производится по паре логин/пароль.
// Каждый логин должен быть уникальным.
// Возможные ошибки
// ErrLoginTaken — логин уже занят;
func (s *service) Register(ctx context.Context, login, pass string) (string, error) {
	hash := sha256.Sum256([]byte(pass))
	hashStr := hex.EncodeToString(hash[:])

	u4 := uuid.NewString()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO public.account 
            (account_login, account_pass_sha256, account_uuid) 
         VALUES ($1, $2, $3)`,
		login, hashStr, u4,
	)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			return "", ErrLoginTaken
		}
		return "", err
	}
	return u4, nil
}

// Login — поиск пользователя. поиск производится по паре логин/хэш.
// Возможные ошибки
// ErrUserNotFound — пользователь не найден;
func (s *service) Login(ctx context.Context, login, pass string) (string, error) {
	hash := sha256.Sum256([]byte(pass))
	hashStr := fmt.Sprintf("%x", hash)

	var _accid string
	err := s.db.QueryRowContext(ctx,
		`SELECT account_uuid
		 FROM public.account 
		 WHERE account_login = $1 AND account_pass_sha256 = $2`,
		login, hashStr,
	).Scan(&_accid)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", ErrUserNotFound
		}
		return "", err
	}

	return _accid, nil
}

// InsertOrder - вставка номера заказа для расчёта.
// Номером заказа является последовательность цифр произвольной длины.
// Возможные ошибки:
// ErrOrderAlreadyExists — номер заказа уже был загружен этим пользователем;
// ErrOrderTakenByAnother — номер заказа уже был загружен другим пользователем;
// ErrInvalidOrderFormat — неверный формат номера заказа;
func (s *service) InsertOrder(ctx context.Context, accountUUID string, orderCode int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO public.orders (orders_uuid, orders_account_uuid, orders_code)
		 VALUES (gen_random_uuid(), $1, $2)`,
		accountUUID, orderCode,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation {
			var ownerUUID string
			err := s.db.QueryRowContext(ctx,
				`SELECT orders_account_uuid FROM public.orders WHERE orders_code = $1`,
				orderCode,
			).Scan(&ownerUUID)
			if err != nil {
				return err
			}

			if ownerUUID == accountUUID {
				return ErrOrderAlreadyExists
			}
			return ErrOrderTakenByAnother
		}
		return err
	}

	return nil
}

// GetOrdersByAccountUUID — получение списка загруженных пользователем номеров заказов, статусов их обработки
// и информации о начислениях
func (s *service) GetOrdersByAccountUUID(ctx context.Context, accountUUID string) ([]map[string]interface{}, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT orders_code, orders_create_ts, orders_status, orders_accrual
		 FROM public.orders
		 WHERE orders_account_uuid = $1
		 ORDER BY orders_create_ts DESC`,
		accountUUID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []map[string]interface{}
	for rows.Next() {
		var code int64
		var createdAt time.Time
		var status string
		var accrual decimal.NullDecimal

		err := rows.Scan(&code, &createdAt, &status, &accrual)
		if err != nil {
			return nil, err
		}

		order := map[string]interface{}{
			"number":      strconv.FormatInt(code, 10),
			"uploaded_at": createdAt,
			"status":      status,
		}

		if accrual.Valid {
			order["accrual"] = accrual.Decimal
		}
		orders = append(orders, order)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return orders, nil
}

// GetBalance — получение информации о балансе пользователя.
// Возможные ошибки:
// ErrAccountNotFound — аккаунт не найден.
func (s *service) GetBalance(ctx context.Context, accountUUID string) (map[string]interface{}, error) {
	var balance decimal.Decimal
	var withdrawn decimal.Decimal
	err := s.db.QueryRowContext(ctx,
		`SELECT account_balance, COALESCE(cnt, 0)
		 FROM public.account
		 inner join (select sum(w.withdrawals_sum) cnt from withdrawals w
		 where w.withdrawals_account_uuid = $1)k1 on true
		 WHERE account_uuid = $1`,
		accountUUID,
	).Scan(&balance, &withdrawn)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAccountNotFound
		}
		return nil, err
	}

	return map[string]interface{}{
		"current":   balance.InexactFloat64(),
		"withdrawn": withdrawn.InexactFloat64(),
	}, nil
}

// Withdraw — списание баллов с накопительного счёта пользователя.
// Возможные ошибки:
// ErrAccountNotFound — аккаунт не найден;
// ErrInsufficientFunds — недостаточно средств на счету.
func (s *service) Withdraw(ctx context.Context, accountUUID string, orderNumber int64, sum decimal.Decimal) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var balance decimal.Decimal
	err = tx.QueryRowContext(ctx,
		`SELECT account_balance
		 FROM public.account
		 WHERE account_uuid = $1
		 FOR UPDATE`,
		accountUUID,
	).Scan(&balance)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAccountNotFound
		}
		return err
	}

	if balance.LessThan(sum) {
		return ErrInsufficientFunds // 402
	}

	_, err = tx.ExecContext(ctx,
		`UPDATE public.account
		 SET account_balance = account_balance - $1
		 WHERE account_uuid = $2`,
		sum, accountUUID,
	)
	if err != nil {
		return err
	}

	_, err = tx.ExecContext(ctx,
		`INSERT INTO public.withdrawals 
			(withdrawals_uuid, withdrawals_account_uuid, withdrawals_sum, withdrawals_order_number)
		 VALUES (gen_random_uuid(), $1, $2, $3)`,
		accountUUID, sum, orderNumber,
	)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// DB — возвращает внутренний *sql.DB для воркера.
func (s *service) DB() *sql.DB {
	return s.db
}

// GetWithdrawals — получение информации о выводе средств с накопительного счёта пользователем.
func (s *service) GetWithdrawals(ctx context.Context, accountUUID string) ([]map[string]interface{}, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT withdrawals_order_number, withdrawals_sum, withdrawals_create_ts
		 FROM public.withdrawals
		 WHERE withdrawals_account_uuid = $1
		 ORDER BY withdrawals_create_ts DESC`,
		accountUUID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var withdrawals []map[string]interface{}
	for rows.Next() {
		var orderNumber int64
		var sum decimal.Decimal
		var createdAt time.Time

		err := rows.Scan(&orderNumber, &sum, &createdAt)
		if err != nil {
			return nil, err
		}

		withdrawals = append(withdrawals, map[string]interface{}{
			"order":        strconv.FormatInt(orderNumber, 10),
			"sum":          sum,
			"processed_at": createdAt,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return withdrawals, nil
}

// Close — закрытие соединения с базой данных.
func (s *service) Close() error {
	log.Printf("Disconnected from database")
	return s.db.Close()
}
