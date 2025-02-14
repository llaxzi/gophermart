package repository

import (
	"context"
	"errors"
	"fmt"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
	"github.com/llaxzi/gophermart/internal/apperrors"
	"github.com/llaxzi/gophermart/internal/models"
	"time"

	"github.com/stretchr/testify/assert"
	"testing"
)

func TestInsertUser(t *testing.T) {
	// Создаем мок БД
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	// Создаем экземпляр репозитория
	repo := repository{db: db}

	// Тестовые данные
	user := models.User{
		Login:    "testuser",
		Password: "hashedpassword",
	}

	// Тестовые сценарии
	tests := []struct {
		name          string
		mockBehavior  func()
		expectedError error
	}{
		{
			name: "Successful user insertion",
			mockBehavior: func() {
				mock.ExpectExec(`INSERT INTO gophermart\.users\(login,password,balance_current,balance_withdrawn\) VALUES \(\$1,\$2,\$3,\$4\)`).
					WithArgs(user.Login, user.Password, 0, 0).
					WillReturnResult(sqlmock.NewResult(1, 1)) // ID = 1, затронуто 1 строка
			},
			expectedError: nil,
		},
		{
			name: "Duplicate login error",
			mockBehavior: func() {
				mock.ExpectExec(`INSERT INTO gophermart\.users\(login,password,balance_current,balance_withdrawn\) VALUES \(\$1,\$2,\$3,\$4\)`).
					WithArgs(user.Login, user.Password, 0, 0).
					WillReturnError(&pgconn.PgError{Code: "23505"}) // Симулируем ошибку уникальности
			},
			expectedError: apperrors.ErrLoginTaken,
		},
		{
			name: "Database connection error",
			mockBehavior: func() {
				mock.ExpectExec(`INSERT INTO gophermart\.users\(login,password,balance_current,balance_withdrawn\) VALUES \(\$1,\$2,\$3,\$4\)`).
					WithArgs(user.Login, user.Password, 0, 0).
					WillReturnError(&pgconn.PgError{Code: "08006"}) // Симулируем ошибку соединения
			},
			expectedError: apperrors.ErrPgConnExc,
		},
		{
			name: "Unexpected database error",
			mockBehavior: func() {
				mock.ExpectExec(`INSERT INTO gophermart\.users\(login,password,balance_current,balance_withdrawn\) VALUES \(\$1,\$2,\$3,\$4\)`).
					WithArgs(user.Login, user.Password, 0, 0).
					WillReturnError(errors.New("some unexpected error")) // Любая другая ошибка
			},
			expectedError: errors.New("some unexpected error"),
		},
	}

	// Запуск тестов
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mockBehavior() // Настраиваем мок

			err = repo.InsertUser(context.Background(), user)

			// Проверяем, что полученная ошибка соответствует ожидаемой
			assert.Equal(t, test.expectedError, err)

			// Проверяем, что все ожидания мока выполнены
			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestWithdrawBalance(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := repository{db: db}

	withdrawal := models.Withdrawal{
		Login:       "testuser",
		Order:       "79927398713",
		Sum:         50.00,
		ProcessedAt: time.Now(),
	}

	tests := []struct {
		name          string
		mockBehavior  func()
		expectedError error
	}{
		{
			name: "Successful withdrawal",
			mockBehavior: func() {
				mock.ExpectBegin()

				mock.ExpectExec(`UPDATE gophermart\.users SET balance_current = balance_current - \$1, balance_withdrawn = balance_withdrawn \+ \$1 WHERE login = \$2 AND balance_current >= \$1`).
					WithArgs(withdrawal.Sum, withdrawal.Login).
					WillReturnResult(sqlmock.NewResult(0, 1))

				mock.ExpectExec(`INSERT INTO gophermart\.withdrawals \(order_id, login, sum, processed_at\) VALUES \(\$1, \$2, \$3, \$4\)`).
					WithArgs(withdrawal.Order, withdrawal.Login, withdrawal.Sum, withdrawal.ProcessedAt).
					WillReturnResult(sqlmock.NewResult(1, 1))

				mock.ExpectCommit()
			},
			expectedError: nil,
		},
		{
			name: "Not enough funds",
			mockBehavior: func() {
				mock.ExpectBegin()

				mock.ExpectExec(`UPDATE gophermart\.users SET balance_current = balance_current - \$1, balance_withdrawn = balance_withdrawn \+ \$1 WHERE login = \$2 AND balance_current >= \$1`).
					WithArgs(withdrawal.Sum, withdrawal.Login).
					WillReturnResult(sqlmock.NewResult(0, 0))

				mock.ExpectRollback()
			},
			expectedError: apperrors.ErrNotEnoughFunds,
		},
		{
			name: "Database connection error during balance update",
			mockBehavior: func() {
				mock.ExpectBegin()

				mock.ExpectExec(`UPDATE gophermart\.users SET balance_current = balance_current - \$1, balance_withdrawn = balance_withdrawn \+ \$1 WHERE login = \$2 AND balance_current >= \$1`).
					WithArgs(withdrawal.Sum, withdrawal.Login).
					WillReturnError(&pgconn.PgError{Code: "08006"})

				mock.ExpectRollback()
			},
			expectedError: apperrors.ErrPgConnExc,
		},
		{
			name: "Database connection error during withdrawal insert",
			mockBehavior: func() {
				mock.ExpectBegin()

				mock.ExpectExec(`UPDATE gophermart\.users SET balance_current = balance_current - \$1, balance_withdrawn = balance_withdrawn \+ \$1 WHERE login = \$2 AND balance_current >= \$1`).
					WithArgs(withdrawal.Sum, withdrawal.Login).
					WillReturnResult(sqlmock.NewResult(0, 1))

				mock.ExpectExec(`INSERT INTO gophermart\.withdrawals \(order_id, login, sum, processed_at\) VALUES \(\$1, \$2, \$3, \$4\)`).
					WithArgs(withdrawal.Order, withdrawal.Login, withdrawal.Sum, withdrawal.ProcessedAt).
					WillReturnError(&pgconn.PgError{Code: "08006"})

				mock.ExpectRollback()
			},
			expectedError: apperrors.ErrPgConnExc,
		},
		{
			name: "Database connection error during transaction commit",
			mockBehavior: func() {
				mock.ExpectBegin()

				mock.ExpectExec(`UPDATE gophermart\.users SET balance_current = balance_current - \$1, balance_withdrawn = balance_withdrawn \+ \$1 WHERE login = \$2 AND balance_current >= \$1`).
					WithArgs(withdrawal.Sum, withdrawal.Login).
					WillReturnResult(sqlmock.NewResult(0, 1))

				mock.ExpectExec(`INSERT INTO gophermart\.withdrawals \(order_id, login, sum, processed_at\) VALUES \(\$1, \$2, \$3, \$4\)`).
					WithArgs(withdrawal.Order, withdrawal.Login, withdrawal.Sum, withdrawal.ProcessedAt).
					WillReturnResult(sqlmock.NewResult(1, 1))

				mock.ExpectCommit().WillReturnError(&pgconn.PgError{Code: "08006"})
			},
			expectedError: apperrors.ErrPgConnExc,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mockBehavior()

			err = repo.WithdrawBalance(context.Background(), withdrawal)

			assert.Equal(t, test.expectedError, err)

			assert.NoError(t, mock.ExpectationsWereMet())
		})
	}
}

func TestSelectNewOrders(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := repository{db: db}

	acr := 10.0

	testOrders := []models.Order{
		{Number: "12345", Login: "user1", Status: "NEW", Accrual: &acr},
		{Number: "67890", Login: "user2", Status: "NEW", Accrual: &acr},
	}

	tests := []struct {
		name          string
		mockBehavior  func()
		expectedError error
		expectedData  []models.Order
	}{
		{
			name: "Successful order selection and update",
			mockBehavior: func() {
				mock.ExpectBegin()

				rows := sqlmock.NewRows([]string{"number", "login", "status", "accrual"}).
					AddRow(testOrders[0].Number, testOrders[0].Login, testOrders[0].Status, *(testOrders[0].Accrual)).
					AddRow(testOrders[1].Number, testOrders[1].Login, testOrders[1].Status, *(testOrders[1].Accrual))
				mock.ExpectQuery(`SELECT number,login, status, accrual FROM gophermart\.orders WHERE status = 'NEW' ORDER BY uploaded_at FOR UPDATE SKIP LOCKED`).
					WillReturnRows(rows)

				mock.ExpectExec(`UPDATE gophermart\.orders SET status = 'PROCESSING' where number = ANY\(\$1\)`).
					WithArgs(pq.Array([]string{testOrders[0].Number, testOrders[1].Number})).
					WillReturnResult(sqlmock.NewResult(0, 2))

				mock.ExpectCommit()
			},
			expectedError: nil,
			expectedData:  testOrders,
		},
		{
			name: "No new orders",
			mockBehavior: func() {
				mock.ExpectBegin()

				rows := sqlmock.NewRows([]string{"number", "login", "status", "accrual"})
				mock.ExpectQuery(`SELECT number,login, status, accrual FROM gophermart\.orders WHERE status = 'NEW' ORDER BY uploaded_at FOR UPDATE SKIP LOCKED`).
					WillReturnRows(rows)

				mock.ExpectCommit()
			},
			expectedError: nil,
			expectedData:  []models.Order{},
		},
		{
			name: "Database connection error on Begin",
			mockBehavior: func() {
				mock.ExpectBegin().WillReturnError(&pgconn.PgError{Code: "08006"})
			},
			expectedError: apperrors.ErrPgConnExc,
			expectedData:  nil,
		},
		{
			name: "Database error during SELECT",
			mockBehavior: func() {
				mock.ExpectBegin()

				mock.ExpectQuery(`SELECT number,login, status, accrual FROM gophermart\.orders WHERE status = 'NEW' ORDER BY uploaded_at FOR UPDATE SKIP LOCKED`).
					WillReturnError(&pgconn.PgError{Code: "08006"})
			},
			expectedError: apperrors.ErrPgConnExc,
			expectedData:  nil,
		},
		{
			name: "Error while scanning rows",
			mockBehavior: func() {
				mock.ExpectBegin() // ✅ Начало транзакции

				// ❌ Ошибка при `Scan`
				rows := sqlmock.NewRows([]string{"number", "login", "status", "accrual"}).
					AddRow("12345", "testuser", "NEW", "invalid_number") // Передаём некорректный `accrual`

				mock.ExpectQuery(`SELECT number,login, status, accrual FROM gophermart\.orders WHERE status = 'NEW' ORDER BY uploaded_at FOR UPDATE SKIP LOCKED`).
					WillReturnRows(rows)
			},
			expectedError: fmt.Errorf("sql: Scan error on column index 3, name \"accrual\": converting driver.Value type string (\"invalid_number\") to a float64: invalid syntax"),
			expectedData:  nil,
		},
		{
			name: "Database error during UPDATE",
			mockBehavior: func() {
				mock.ExpectBegin()

				rows := sqlmock.NewRows([]string{"number", "login", "status", "accrual"}).
					AddRow(testOrders[0].Number, testOrders[0].Login, testOrders[0].Status, testOrders[0].Accrual)
				mock.ExpectQuery(`SELECT number,login, status, accrual FROM gophermart\.orders WHERE status = 'NEW' ORDER BY uploaded_at FOR UPDATE SKIP LOCKED`).
					WillReturnRows(rows)

				mock.ExpectExec(`UPDATE gophermart\.orders SET status = 'PROCESSING' where number = ANY\(\$1\)`).
					WithArgs(pq.Array([]string{testOrders[0].Number})).
					WillReturnError(&pgconn.PgError{Code: "08006"})
			},
			expectedError: apperrors.ErrPgConnExc,
			expectedData:  nil,
		},
		{
			name: "Database error during COMMIT",
			mockBehavior: func() {
				mock.ExpectBegin()

				rows := sqlmock.NewRows([]string{"number", "login", "status", "accrual"}).
					AddRow(testOrders[0].Number, testOrders[0].Login, testOrders[0].Status, testOrders[0].Accrual)
				mock.ExpectQuery(`SELECT number,login, status, accrual FROM gophermart\.orders WHERE status = 'NEW' ORDER BY uploaded_at FOR UPDATE SKIP LOCKED`).
					WillReturnRows(rows)

				mock.ExpectExec(`UPDATE gophermart\.orders SET status = 'PROCESSING' where number = ANY\(\$1\)`).
					WithArgs(pq.Array([]string{testOrders[0].Number})).
					WillReturnResult(sqlmock.NewResult(0, 1))

				mock.ExpectCommit().WillReturnError(&pgconn.PgError{Code: "08006"})
			},
			expectedError: apperrors.ErrPgConnExc,
			expectedData:  nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mockBehavior()

			orders, err := repo.SelectNewOrders(context.Background())

			if test.expectedError != nil {
				assert.Error(t, err, "expected error but got nil")

				if test.name == "Error while scanning rows" {
					// Проверяем, что ошибка содержит нужный текст, независимо от обёртки
					assert.Contains(t, err.Error(), "sql: Scan error on column index 3", "unexpected error in test: %s", test.name)
				} else {
					// Проверяем точное совпадение ошибки
					assert.Equal(t, test.expectedError, err, "unexpected error in test: %s", test.name)
				}
			} else {
				assert.NoError(t, err, "unexpected error in test: %s", test.name)
			}

			assert.Equal(t, test.expectedData, orders, "unexpected data in test: %s", test.name)
			assert.NoError(t, mock.ExpectationsWereMet(), "unexpected sqlmock expectations failure in test: %s", test.name)
		})
	}

}

func TestUpdateOrder(t *testing.T) {
	db, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer db.Close()

	repo := repository{db: db}

	accrual := 10.0

	tests := []struct {
		name          string
		order         models.Order
		mockBehavior  func()
		expectedError error
	}{
		{
			name: "Successful order update",
			order: models.Order{
				Number:  "12345",
				Login:   "testuser",
				Status:  "PROCESSED",
				Accrual: &accrual,
			},
			mockBehavior: func() {
				mock.ExpectBegin()

				mock.ExpectExec(`UPDATE gophermart\.orders SET status = \$1, accrual = \$2 WHERE number = \$3`).
					WithArgs("PROCESSED", &accrual, "12345").
					WillReturnResult(sqlmock.NewResult(0, 1))

				mock.ExpectExec(`UPDATE gophermart\.users SET balance_current = balance_current \+ \$1 WHERE login = \$2 RETURNING balance_current`).
					WithArgs(&accrual, "testuser").
					WillReturnResult(sqlmock.NewResult(0, 1))

				mock.ExpectCommit()
			},
			expectedError: nil,
		},
		{
			name: "Successful order update without balance update",
			order: models.Order{
				Number:  "12345",
				Login:   "testuser",
				Status:  "NEW",
				Accrual: &accrual,
			},
			mockBehavior: func() {
				mock.ExpectBegin()

				mock.ExpectExec(`UPDATE gophermart\.orders SET status = \$1, accrual = \$2 WHERE number = \$3`).
					WithArgs("NEW", &accrual, "12345").
					WillReturnResult(sqlmock.NewResult(0, 1))

				mock.ExpectCommit()
			},
			expectedError: nil,
		},
		{
			name:  "Database connection error on Begin",
			order: models.Order{},
			mockBehavior: func() {
				mock.ExpectBegin().WillReturnError(&pgconn.PgError{Code: "08006"})
			},
			expectedError: apperrors.ErrPgConnExc,
		},
		{
			name: "Database error during order update",
			order: models.Order{
				Number:  "12345",
				Status:  "NEW",
				Accrual: &accrual,
			},
			mockBehavior: func() {
				mock.ExpectBegin()

				mock.ExpectExec(`UPDATE gophermart\.orders SET status = \$1, accrual = \$2 WHERE number = \$3`).
					WithArgs("NEW", &accrual, "12345").
					WillReturnError(&pgconn.PgError{Code: "08006"})
			},
			expectedError: apperrors.ErrPgConnExc,
		},
		{
			name: "Database error during user balance update",
			order: models.Order{
				Number:  "12345",
				Login:   "testuser",
				Status:  "PROCESSED",
				Accrual: &accrual,
			},
			mockBehavior: func() {
				mock.ExpectBegin()

				mock.ExpectExec(`UPDATE gophermart\.orders SET status = \$1, accrual = \$2 WHERE number = \$3`).
					WithArgs("PROCESSED", &accrual, "12345").
					WillReturnResult(sqlmock.NewResult(0, 1))

				mock.ExpectExec(`UPDATE gophermart\.users SET balance_current = balance_current \+ \$1 WHERE login = \$2 RETURNING balance_current`).
					WithArgs(&accrual, "testuser").
					WillReturnError(&pgconn.PgError{Code: "08006"})
			},
			expectedError: apperrors.ErrPgConnExc,
		},
		{
			name: "Database error during COMMIT",
			order: models.Order{
				Number:  "12345",
				Login:   "testuser",
				Status:  "PROCESSED",
				Accrual: &accrual,
			},
			mockBehavior: func() {
				mock.ExpectBegin()

				mock.ExpectExec(`UPDATE gophermart\.orders SET status = \$1, accrual = \$2 WHERE number = \$3`).
					WithArgs("PROCESSED", &accrual, "12345").
					WillReturnResult(sqlmock.NewResult(0, 1))

				mock.ExpectExec(`UPDATE gophermart\.users SET balance_current = balance_current \+ \$1 WHERE login = \$2 RETURNING balance_current`).
					WithArgs(&accrual, "testuser").
					WillReturnResult(sqlmock.NewResult(0, 1))

				mock.ExpectCommit().WillReturnError(&pgconn.PgError{Code: "08006"})
			},
			expectedError: apperrors.ErrPgConnExc,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.mockBehavior()

			err := repo.UpdateOrder(context.Background(), test.order)

			if test.expectedError != nil {
				assert.Error(t, err, "expected error but got nil")
				assert.Equal(t, test.expectedError, err, "unexpected error in test: %s", test.name)
			} else {
				assert.NoError(t, err, "unexpected error in test: %s", test.name)
			}

			assert.NoError(t, mock.ExpectationsWereMet(), "unexpected sqlmock expectations failure in test: %s", test.name)
		})
	}
}
