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
	defer rows.Close()
	if err != nil {
		if r.isPgConnErr(err) {
			return nil, apperrors.ErrPgConnExc
		}
		return nil, err
	}

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
