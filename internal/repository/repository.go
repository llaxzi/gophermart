package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/lib/pq"
	"github.com/llaxzi/gophermart/internal/apperrors"
	"github.com/llaxzi/gophermart/internal/models"
	"time"
)

type Repository interface {
	InsertUser(ctx context.Context, user models.User) error
	SelectUser(ctx context.Context, userLogin string) (string, error)
	InsertOrder(ctx context.Context, order models.Order) error
	SelectOrders(ctx context.Context, userLogin string) ([]models.OrderResponse, error)
	SelectBalance(ctx context.Context, userLogin string) (models.Balance, error)
	WithdrawBalance(ctx context.Context, withdrawal models.Withdrawal) error
	SelectWithdrawals(ctx context.Context, userLogin string) ([]models.WithdrawalResponse, error)
	SelectNewOrders(ctx context.Context) ([]models.Order, error)
	UpdateOrder(ctx context.Context, order models.Order) error
	ResetStatus(ctx context.Context, orderNumber string) error
	Bootstrap(dsn string, steps int) error
}

func NewRepository(dsn string) (Repository, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	return &repository{db}, nil
}

type repository struct {
	db *sql.DB
}

func (r *repository) InsertUser(ctx context.Context, user models.User) error {
	query := "INSERT INTO gophermart.users(login,password,balance_current,balance_withdrawn) VALUES ($1,$2,$3,$4)"
	_, err := r.db.ExecContext(ctx, query, user.Login, user.Password, 0, 0)

	if r.isPgConnErr(err) {
		return apperrors.ErrPgConnExc
	}
	if r.isPgUniqueViolationErr(err) {
		return apperrors.ErrLoginTaken
	}

	return err
}

func (r *repository) SelectUser(ctx context.Context, userLogin string) (string, error) {
	var password string
	query := "SELECT password FROM gophermart.users WHERE login = $1"

	if err := r.db.QueryRowContext(ctx, query, userLogin).Scan(&password); err != nil {
		if r.isPgConnErr(err) {
			return "", apperrors.ErrPgConnExc
		}
		if errors.Is(err, sql.ErrNoRows) {
			return "", apperrors.ErrInvalidLP
		}
	}
	return password, nil
}

func (r *repository) InsertOrder(ctx context.Context, order models.Order) error {
	query := "INSERT INTO gophermart.orders(number, login, status, uploaded_at) VALUES ($1,$2,$3,$4)"
	_, err := r.db.ExecContext(ctx, query, order.Number, order.Login, order.Status, order.UploadedAt)
	if r.isPgConnErr(err) {
		return apperrors.ErrPgConnExc
	}
	if !r.isPgUniqueViolationErr(err) {
		return err
	}

	var orderLogin string
	query = "SELECT login FROM gophermart.orders WHERE number = $1"
	err = r.db.QueryRowContext(ctx, query, order.Number).Scan(&orderLogin)
	if err != nil {
		return err
	}

	if orderLogin == order.Login {
		return apperrors.ErrOrderInserted
	}
	return apperrors.ErrOrderInsertedLogin
}

func (r *repository) SelectOrders(ctx context.Context, userLogin string) ([]models.OrderResponse, error) {
	query := "SELECT number,status,accrual,uploaded_at FROM gophermart.orders WHERE login = $1 ORDER BY uploaded_at DESC"
	rows, err := r.db.QueryContext(ctx, query, userLogin)
	if err != nil {
		if r.isPgConnErr(err) {
			return nil, apperrors.ErrPgConnExc
		}
		return nil, err
	}
	defer rows.Close()

	var orders []models.OrderResponse
	for rows.Next() {
		var order models.OrderResponse
		var accr sql.NullFloat64
		var uploadedAt time.Time
		if err = rows.Scan(&order.Number, &order.Status, &accr, &uploadedAt); err != nil {
			return orders, err
		}
		if accr.Valid {
			order.Accrual = &accr.Float64
		}
		order.UploadedAt = uploadedAt.Format(time.RFC3339)
		orders = append(orders, order)
	}
	if err = rows.Err(); err != nil {
		return orders, err
	}
	return orders, nil
}

func (r *repository) SelectBalance(ctx context.Context, userLogin string) (models.Balance, error) {
	query := "SELECT balance_current, balance_withdrawn FROM gophermart.users WHERE login = $1"
	var balance models.Balance
	err := r.db.QueryRowContext(ctx, query, userLogin).Scan(&balance.Current, &balance.Withdrawn)

	if r.isPgConnErr(err) {
		return balance, apperrors.ErrPgConnExc
	}

	return balance, err

}

func (r *repository) WithdrawBalance(ctx context.Context, withdrawal models.Withdrawal) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		if r.isPgConnErr(err) {
			return apperrors.ErrPgConnExc
		}
		return err
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	query := "UPDATE gophermart.users SET balance_current = balance_current - $1, balance_withdrawn = balance_withdrawn + $1 WHERE login = $2 AND balance_current >= $1"
	res, err := tx.ExecContext(ctx, query, withdrawal.Sum, withdrawal.Login)
	if err != nil {
		if r.isPgConnErr(err) {
			return apperrors.ErrPgConnExc
		}
		return err
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		if r.isPgConnErr(err) {
			return apperrors.ErrPgConnExc
		}
		return err
	}

	if rowsAffected == 0 {
		return apperrors.ErrNotEnoughFunds
	}

	query = "INSERT INTO gophermart.withdrawals (order_id, login, sum, processed_at)  VALUES ($1, $2, $3, $4)"
	_, err = tx.ExecContext(ctx, query, withdrawal.Order, withdrawal.Login, withdrawal.Sum, withdrawal.ProcessedAt)
	if err != nil {
		if r.isPgConnErr(err) {
			return apperrors.ErrPgConnExc
		}
		return err
	}

	err = tx.Commit()
	if err != nil {
		if r.isPgConnErr(err) {
			return apperrors.ErrPgConnExc
		}
		return err
	}
	return nil
}

func (r *repository) SelectNewOrders(ctx context.Context) ([]models.Order, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		if r.isPgConnErr(err) {
			return nil, apperrors.ErrPgConnExc
		}
		return nil, err
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	query := "SELECT number,login, status, accrual FROM gophermart.orders WHERE status = 'NEW' ORDER BY uploaded_at FOR UPDATE SKIP LOCKED"
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		if r.isPgConnErr(err) {
			return nil, apperrors.ErrPgConnExc
		}
		return nil, err
	}
	defer rows.Close()

	var orders []models.Order
	var orderNumbers []string
	for rows.Next() {
		var order models.Order
		if err = rows.Scan(&order.Number, &order.Login, &order.Status, &order.Accrual); err != nil {
			return orders, err
		}
		orders = append(orders, order)
		orderNumbers = append(orderNumbers, order.Number)
	}
	if err = rows.Err(); err != nil {
		return orders, err
	}

	if len(orders) < 1 {
		return orders, nil
	}

	updateQuery := "UPDATE gophermart.orders SET status = 'PROCESSING' where number = ANY($1)"
	_, err = tx.ExecContext(ctx, updateQuery, pq.Array(orderNumbers))
	if err != nil {
		if r.isPgConnErr(err) {
			return nil, apperrors.ErrPgConnExc
		}
		return orders, err
	}

	if err = tx.Commit(); err != nil {
		if r.isPgConnErr(err) {
			return nil, apperrors.ErrPgConnExc
		}
		return nil, err
	}

	return orders, nil
}

func (r *repository) UpdateOrder(ctx context.Context, order models.Order) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		if r.isPgConnErr(err) {
			return apperrors.ErrPgConnExc
		}
		return err
	}

	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	query := "UPDATE gophermart.orders SET status = $1, accrual = $2 WHERE number = $3"
	_, err = tx.ExecContext(ctx, query, order.Status, order.Accrual, order.Number)
	if err != nil {
		if r.isPgConnErr(err) {
			return apperrors.ErrPgConnExc
		}
		return err
	}

	if order.Status == "PROCESSED" {
		query = "UPDATE gophermart.users SET balance_current = balance_current + $1 WHERE login = $2 RETURNING balance_current"
		_, err = tx.ExecContext(ctx, query, order.Accrual, order.Login)
		if err != nil {
			if r.isPgConnErr(err) {
				return apperrors.ErrPgConnExc
			}
			return err
		}
	}

	if err = tx.Commit(); err != nil {
		if r.isPgConnErr(err) {
			return apperrors.ErrPgConnExc
		}
		return err
	}

	return nil
}

func (r *repository) ResetStatus(ctx context.Context, orderNumber string) error {
	query := "UPDATE gophermart.orders SET status = 'NEW' WHERE number = $1"

	_, err := r.db.ExecContext(ctx, query, orderNumber)
	if r.isPgConnErr(err) {
		return apperrors.ErrPgConnExc
	}
	return err
}

func (r *repository) Bootstrap(dsn string, steps int) error {
	m, err := migrate.New("file://internal/migrations", dsn)
	if err != nil {
		return fmt.Errorf("failed to create migrator: %w", err)
	}

	// Запускаем все миграции
	if steps == 0 {
		err = m.Up()
	} else { // Запускаем steps шагов, в том числе < 0
		err = m.Steps(steps)
	}

	if err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migrations failed: %w", err)
	}
	return nil
}

func (r *repository) isPgConnErr(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgerrcode.IsConnectionException(pgErr.Code)
}

func (r *repository) isPgUniqueViolationErr(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgerrcode.UniqueViolation
}

func (r *repository) SelectWithdrawals(ctx context.Context, userLogin string) ([]models.WithdrawalResponse, error) {
	query := "SELECT order_id,sum,processed_at FROM gophermart.withdrawals WHERE login = $1 ORDER BY processed_at DESC"
	rows, err := r.db.QueryContext(ctx, query, userLogin)
	if err != nil {
		if r.isPgConnErr(err) {
			return nil, apperrors.ErrPgConnExc
		}
		return nil, err
	}
	defer rows.Close()

	var withdrawals []models.WithdrawalResponse
	for rows.Next() {
		var withdrawal models.WithdrawalResponse
		var processedAt time.Time
		if err = rows.Scan(&withdrawal.Order, &withdrawal.Sum, &processedAt); err != nil {
			return withdrawals, err
		}

		withdrawal.ProcessedAt = processedAt.Format(time.RFC3339)
		withdrawals = append(withdrawals, withdrawal)
	}
	if err = rows.Err(); err != nil {
		return withdrawals, err
	}
	return withdrawals, nil
}
