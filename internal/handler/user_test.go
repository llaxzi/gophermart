package handler_test

import (
	"bytes"
	"encoding/json"
	"github.com/golang/mock/gomock"
	"github.com/labstack/echo/v4"
	"github.com/llaxzi/gophermart/internal/apperrors"
	"github.com/llaxzi/gophermart/internal/handler"
	"github.com/llaxzi/gophermart/internal/mocks"
	"github.com/llaxzi/gophermart/internal/models"
	"github.com/llaxzi/retryables/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestUserRegister(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockRepository(ctrl)
	tokenBuilder := mocks.NewMockTokenBuilder(ctrl) // Мокаем TokenBuilder
	retryer := retryables.NewRetryer(nil)
	retryer.SetCount(1)

	h := handler.NewUserHandler(repo, tokenBuilder, retryer)

	tests := []struct {
		name           string
		inputUser      models.User
		repoError      error
		expectRepoCall bool
		expectJWTCall  bool
		expectedStatus int
	}{
		{
			name: "Successful registration",
			inputUser: models.User{
				Login:    "testuser",
				Password: "password123",
			},
			repoError:      nil,
			expectRepoCall: true,
			expectJWTCall:  true,
			expectedStatus: http.StatusOK,
		},
		{
			name: "Conflict - username already taken",
			inputUser: models.User{
				Login:    "existing_user",
				Password: "password123",
			},
			repoError:      apperrors.ErrLoginTaken,
			expectRepoCall: true,
			expectJWTCall:  false,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "Invalid JSON",
			inputUser:      models.User{},
			repoError:      nil,
			expectRepoCall: false,
			expectJWTCall:  false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Internal server error",
			inputUser: models.User{
				Login:    "server_error_user",
				Password: "password123",
			},
			repoError:      apperrors.ErrServer,
			expectRepoCall: true,
			expectJWTCall:  false,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := echo.New()

			var userJSON []byte
			if test.name == "Invalid JSON" {
				userJSON = []byte("{invalid-json}")
			} else {
				userJSON, _ = json.Marshal(test.inputUser)
			}

			req := httptest.NewRequest(http.MethodPost, "/register", bytes.NewReader(userJSON))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)

			if test.expectRepoCall {
				repo.EXPECT().
					InsertUser(gomock.Any(), gomock.Any()).
					Return(test.repoError).
					Times(1)
			}

			if test.expectJWTCall {
				tokenBuilder.EXPECT().
					BuildJWTString(gomock.Any()).
					Return("mock_token", nil).
					Times(1)
			}

			err := h.Register(ctx)

			require.NoError(t, err)
			assert.Equal(t, test.expectedStatus, rec.Code, "Expected status %d but got %d", test.expectedStatus, rec.Code)
		})
	}
}

func TestUserLogin(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockRepository(ctrl)
	tokenBuilder := mocks.NewMockTokenBuilder(ctrl)
	retryer := retryables.NewRetryer(nil)
	retryer.SetCount(1)

	h := handler.NewUserHandler(repo, tokenBuilder, retryer)

	password := "password123"
	hashedPassword, _ := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	tests := []struct {
		name           string
		inputUser      models.User
		repoError      error
		tokenError     error
		expectRepoCall bool
		expectJWTCall  bool
		expectedStatus int
		returnedHash   string
	}{
		{
			name: "Successful login",
			inputUser: models.User{
				Login:    "testuser",
				Password: password,
			},
			repoError:      nil,
			tokenError:     nil,
			expectRepoCall: true,
			expectJWTCall:  true,
			expectedStatus: http.StatusOK,
			returnedHash:   string(hashedPassword),
		},
		{
			name: "Invalid JSON",
			inputUser: models.User{
				Login:    "",
				Password: "",
			},
			repoError:      nil,
			tokenError:     nil,
			expectRepoCall: false,
			expectJWTCall:  false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "User not found",
			inputUser: models.User{
				Login:    "unknown_user",
				Password: password,
			},
			repoError:      apperrors.ErrInvalidLP,
			tokenError:     nil,
			expectRepoCall: true,
			expectJWTCall:  false,
			expectedStatus: http.StatusUnauthorized,
			returnedHash:   "",
		},
		{
			name: "Incorrect password",
			inputUser: models.User{
				Login:    "testuser",
				Password: "wrongpassword",
			},
			repoError:      nil,
			tokenError:     nil,
			expectRepoCall: true,
			expectJWTCall:  false,
			expectedStatus: http.StatusUnauthorized,
			returnedHash:   string(hashedPassword),
		},
		{
			name: "Server error when retrieving password",
			inputUser: models.User{
				Login:    "server_error_user",
				Password: password,
			},
			repoError:      apperrors.ErrServer,
			tokenError:     nil,
			expectRepoCall: true,
			expectJWTCall:  false,
			expectedStatus: http.StatusInternalServerError,
			returnedHash:   "",
		},
		{
			name: "Error generating JWT token",
			inputUser: models.User{
				Login:    "testuser",
				Password: password,
			},
			repoError:      nil,
			tokenError:     apperrors.ErrServer,
			expectRepoCall: true,
			expectJWTCall:  true,
			expectedStatus: http.StatusInternalServerError,
			returnedHash:   string(hashedPassword),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := echo.New()

			var userJSON []byte
			if test.name == "Invalid JSON" {
				userJSON = []byte("{invalid-json}")
			} else {
				userJSON, _ = json.Marshal(test.inputUser)
			}

			req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(userJSON))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)

			if test.expectRepoCall {
				repo.EXPECT().
					SelectUser(gomock.Any(), test.inputUser.Login).
					Return(test.returnedHash, test.repoError).
					Times(1)
			}

			if test.expectJWTCall {
				tokenBuilder.EXPECT().
					BuildJWTString(test.inputUser.Login).
					Return("mock_token", test.tokenError).
					Times(1)
			}

			err := h.Login(ctx)

			require.NoError(t, err)
			assert.Equal(t, test.expectedStatus, rec.Code, "Expected status %d but got %d", test.expectedStatus, rec.Code)
		})
	}
}

func TestAddOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockRepository(ctrl)
	retryer := retryables.NewRetryer(nil)
	retryer.SetCount(1)

	h := handler.NewUserHandler(repo, nil, retryer)

	tests := []struct {
		name           string
		orderNumber    string
		repoError      error
		expectRepoCall bool
		expectedStatus int
	}{
		{
			name:           "Successful order addition",
			orderNumber:    "79927398713", // Корректный номер по алгоритму Луна
			repoError:      nil,
			expectRepoCall: true,
			expectedStatus: http.StatusAccepted,
		},
		{
			name:           "Invalid JSON",
			orderNumber:    "",
			repoError:      nil,
			expectRepoCall: false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "Invalid Luhn number",
			orderNumber:    "123456789", // Некорректный номер заказа
			repoError:      nil,
			expectRepoCall: false,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name:           "Order already exists for another user",
			orderNumber:    "79927398713",
			repoError:      apperrors.ErrOrderInsertedLogin,
			expectRepoCall: true,
			expectedStatus: http.StatusConflict,
		},
		{
			name:           "Order already exists for the same user",
			orderNumber:    "79927398713",
			repoError:      apperrors.ErrOrderInserted,
			expectRepoCall: true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Database error",
			orderNumber:    "79927398713",
			repoError:      apperrors.ErrServer,
			expectRepoCall: true,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := echo.New()

			var orderBody []byte
			if test.name == "Invalid JSON" {
				orderBody = []byte("{invalid-json}")
			} else {
				orderBody = []byte(test.orderNumber)
			}

			req := httptest.NewRequest(http.MethodPost, "/order", bytes.NewReader(orderBody))
			req.Header.Set("Content-Type", "text/plain")
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)

			ctx.Set("user_login", "testuser")

			if test.expectRepoCall {
				repo.EXPECT().
					InsertOrder(gomock.Any(), gomock.Any()).
					Return(test.repoError).
					Times(1)
			}

			err := h.AddOrder(ctx)

			require.NoError(t, err)
			assert.Equal(t, test.expectedStatus, rec.Code, "Expected status %d but got %d", test.expectedStatus, rec.Code)
		})
	}
}

func TestGetOrders(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockRepository(ctrl)
	retryer := retryables.NewRetryer(nil)
	retryer.SetCount(1)

	h := handler.NewUserHandler(repo, nil, retryer)

	tests := []struct {
		name           string
		orders         []models.OrderResponse
		repoError      error
		expectedStatus int
	}{
		{
			name: "Successful order retrieval",
			orders: []models.OrderResponse{
				{Number: "79927398713", Status: "PROCESSED", UploadedAt: time.Now().Format(time.RFC3339)},
				{Number: "12345678903", Status: "NEW", UploadedAt: time.Now().Format(time.RFC3339)},
			},
			repoError:      nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "No orders found",
			orders:         []models.OrderResponse{},
			repoError:      nil,
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "Database error",
			orders:         nil,
			repoError:      apperrors.ErrServer,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/orders", nil)
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)

			ctx.Set("user_login", "testuser")

			repo.EXPECT().
				SelectOrders(gomock.Any(), "testuser").
				Return(test.orders, test.repoError).
				Times(1)

			err := h.GetOrders(ctx)

			require.NoError(t, err)
			assert.Equal(t, test.expectedStatus, rec.Code, "Expected status %d but got %d", test.expectedStatus, rec.Code)

			if test.expectedStatus == http.StatusOK {
				var responseOrders []models.OrderResponse
				err = json.Unmarshal(rec.Body.Bytes(), &responseOrders)
				require.NoError(t, err)
				assert.Equal(t, test.orders, responseOrders)
			}
		})
	}
}

func TestGetBalance(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockRepository(ctrl)
	retryer := retryables.NewRetryer(nil)
	retryer.SetCount(1)

	h := handler.NewUserHandler(repo, nil, retryer)

	tests := []struct {
		name           string
		balance        models.Balance
		repoError      error
		expectedStatus int
	}{
		{
			name: "Successful balance retrieval",
			balance: models.Balance{
				Current:   100.50,
				Withdrawn: 50.00,
			},
			repoError:      nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Database error",
			balance:        models.Balance{},
			repoError:      apperrors.ErrServer,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/balance", nil)
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)

			ctx.Set("user_login", "testuser")

			// Ожидание вызова `SelectBalance`
			repo.EXPECT().
				SelectBalance(gomock.Any(), "testuser").
				Return(test.balance, test.repoError).
				Times(1)

			err := h.GetBalance(ctx)

			require.NoError(t, err)
			assert.Equal(t, test.expectedStatus, rec.Code, "Expected status %d but got %d", test.expectedStatus, rec.Code)

			if test.expectedStatus == http.StatusOK {
				var responseBalance models.Balance
				err = json.Unmarshal(rec.Body.Bytes(), &responseBalance)
				require.NoError(t, err)
				assert.Equal(t, test.balance, responseBalance)
			}
		})
	}
}

func TestWithdraw(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockRepository(ctrl)
	retryer := retryables.NewRetryer(nil)
	retryer.SetCount(1)

	h := handler.NewUserHandler(repo, nil, retryer)

	tests := []struct {
		name           string
		withdrawal     models.Withdrawal
		repoError      error
		expectRepoCall bool
		expectedStatus int
	}{
		{
			name: "Successful withdrawal",
			withdrawal: models.Withdrawal{
				Order: "79927398713",
				Sum:   50.00,
			},
			repoError:      nil,
			expectRepoCall: true,
			expectedStatus: http.StatusOK,
		},
		{
			name: "Invalid JSON (negative sum)",
			withdrawal: models.Withdrawal{
				Order: "79927398713",
				Sum:   -10.00,
			},
			repoError:      nil,
			expectRepoCall: false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Invalid JSON (zero sum)",
			withdrawal: models.Withdrawal{
				Order: "79927398713",
				Sum:   0,
			},
			repoError:      nil,
			expectRepoCall: false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Invalid Luhn number",
			withdrawal: models.Withdrawal{
				Order: "123456789",
				Sum:   50.00,
			},
			repoError:      nil,
			expectRepoCall: false,
			expectedStatus: http.StatusUnprocessableEntity,
		},
		{
			name: "Not enough funds",
			withdrawal: models.Withdrawal{
				Order: "79927398713",
				Sum:   1000.00,
			},
			repoError:      apperrors.ErrNotEnoughFunds,
			expectRepoCall: true,
			expectedStatus: http.StatusPaymentRequired,
		},
		{
			name: "Database error",
			withdrawal: models.Withdrawal{
				Order: "79927398713",
				Sum:   50.00,
			},
			repoError:      apperrors.ErrServer,
			expectRepoCall: true,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := echo.New()

			var withdrawalJSON []byte
			if test.name == "Invalid JSON (negative sum)" || test.name == "Invalid JSON (zero sum)" {
				withdrawalJSON = []byte("{}")
			} else {
				withdrawalJSON, _ = json.Marshal(test.withdrawal)
			}

			req := httptest.NewRequest(http.MethodPost, "/withdraw", bytes.NewReader(withdrawalJSON))
			req.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)

			ctx.Set("user_login", "testuser")

			if test.expectRepoCall {
				repo.EXPECT().
					WithdrawBalance(gomock.Any(), gomock.Any()).
					Return(test.repoError).
					Times(1)
			}

			err := h.Withdraw(ctx)

			require.NoError(t, err)
			assert.Equal(t, test.expectedStatus, rec.Code, "Expected status %d but got %d", test.expectedStatus, rec.Code)
		})
	}
}

func TestGetWithdrawals(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockRepository(ctrl)
	retryer := retryables.NewRetryer(nil)
	retryer.SetCount(1)

	h := handler.NewUserHandler(repo, nil, retryer)

	tests := []struct {
		name           string
		withdrawals    []models.WithdrawalResponse
		repoError      error
		expectedStatus int
	}{
		{
			name: "Successful withdrawals retrieval",
			withdrawals: []models.WithdrawalResponse{
				{Order: "79927398713", Sum: 50.00, ProcessedAt: time.Now().Format(time.RFC3339)},
				{Order: "12345678903", Sum: 25.00, ProcessedAt: time.Now().Format(time.RFC3339)},
			},
			repoError:      nil,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "No withdrawals found",
			withdrawals:    []models.WithdrawalResponse{},
			repoError:      nil,
			expectedStatus: http.StatusNoContent,
		},
		{
			name:           "Database error",
			withdrawals:    nil,
			repoError:      apperrors.ErrServer,
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := echo.New()
			req := httptest.NewRequest(http.MethodGet, "/withdrawals", nil)
			rec := httptest.NewRecorder()
			ctx := e.NewContext(req, rec)

			ctx.Set("user_login", "testuser")

			repo.EXPECT().
				SelectWithdrawals(gomock.Any(), "testuser").
				Return(test.withdrawals, test.repoError).
				Times(1)

			err := h.GetWithdrawals(ctx)

			require.NoError(t, err)
			assert.Equal(t, test.expectedStatus, rec.Code, "Expected status %d but got %d", test.expectedStatus, rec.Code)

			if test.expectedStatus == http.StatusOK {
				var responseWithdrawals []models.WithdrawalResponse
				err := json.Unmarshal(rec.Body.Bytes(), &responseWithdrawals)
				require.NoError(t, err)
				assert.Equal(t, test.withdrawals, responseWithdrawals)
			}
		})
	}
}
