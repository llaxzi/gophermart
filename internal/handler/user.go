package handler

import (
	"errors"
	"github.com/ShiraazMoollatjie/goluhn"
	"github.com/labstack/echo/v4"
	"github.com/llaxzi/gophermart/internal/apperrors"
	"github.com/llaxzi/gophermart/internal/models"
	"github.com/llaxzi/gophermart/internal/repository"
	"github.com/llaxzi/gophermart/internal/tokens"
	"github.com/llaxzi/retryables/v2"
	"golang.org/x/crypto/bcrypt"
	"io"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type UserHandler interface {
	Register(ctx echo.Context) error
	Login(ctx echo.Context) error
	AddOrder(ctx echo.Context) error
	GetOrders(ctx echo.Context) error
	GetBalance(ctx echo.Context) error
	Withdraw(ctx echo.Context) error
	GetWithdrawals(ctx echo.Context) error
}

func NewUserHandler(repo repository.Repository, tokenB tokens.TokenBuilder, retryer *retryables.Retryer) UserHandler {
	return &userHandler{repo, tokenB, retryer}
}

type userHandler struct {
	repo    repository.Repository
	tokenB  tokens.TokenBuilder
	retryer *retryables.Retryer
}

func (h *userHandler) Register(ctx echo.Context) error {
	var user models.User
	err := ctx.Bind(&user)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": apperrors.ErrInvalidJSON.Error()})
	}

	// Хешируем пароль
	hash, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Hash password failed: %v", err)
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": apperrors.ErrServer.Error()})
	}
	user.Password = string(hash)

	err = h.retryer.Retry(func() error {
		return h.repo.InsertUser(ctx.Request().Context(), user)
	})

	if err != nil {
		if errors.Is(err, apperrors.ErrLoginTaken) {
			return ctx.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
		}
		log.Printf("Regiter failed: %v", err)
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": apperrors.ErrServer.Error()})
	}

	// Генерируем JWT токен
	token, err := h.tokenB.BuildJWTString(user.Login)
	if err != nil {
		log.Printf("Failed to generate token: %v for user: %v", err, user.Login)
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": apperrors.ErrServer.Error()})
	}

	ctx.Response().Header().Add("Authorization", "Bearer "+token)

	return ctx.JSON(http.StatusOK, "registered successfully")
}

func (h *userHandler) Login(ctx echo.Context) error {
	var user models.User
	err := ctx.Bind(&user)
	if err != nil {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": apperrors.ErrInvalidJSON.Error()})
	}

	var hashedPassword string

	err = h.retryer.Retry(func() error {
		hashedPassword, err = h.repo.SelectUser(ctx.Request().Context(), user.Login)
		return err
	})
	if err != nil {
		if errors.Is(err, apperrors.ErrInvalidLP) {
			return ctx.JSON(http.StatusUnauthorized, map[string]string{"error": err.Error()})
		}
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": apperrors.ErrServer.Error()})
	}

	err = bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(user.Password))
	if err != nil {
		return ctx.JSON(http.StatusUnauthorized, map[string]string{"error": apperrors.ErrInvalidLP.Error()})
	}

	// Генерируем JWT токен
	token, err := h.tokenB.BuildJWTString(user.Login)
	if err != nil {
		log.Printf("Failed to generate token: %v for user: %v", err, user.Login)
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": apperrors.ErrServer.Error()})
	}

	ctx.Response().Header().Add("Authorization", "Bearer "+token)

	return ctx.JSON(http.StatusOK, "login successfully")
}

func (h *userHandler) AddOrder(ctx echo.Context) error {

	body, err := io.ReadAll(ctx.Request().Body)
	if err != nil || len(body) == 0 {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": apperrors.ErrInvalidJSON.Error()})
	}
	number := strings.TrimSpace(string(body))
	// Тело запроса должно состоять только из цифр
	numericRegex := regexp.MustCompile(`^\d+$`)
	if !numericRegex.MatchString(number) {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": apperrors.ErrInvalidJSON.Error()})
	}

	login := ctx.Get("user_login").(string)
	order := models.Order{
		Number:     number,
		Login:      login,
		Status:     "NEW",
		UploadedAt: time.Now(),
	}

	if err = goluhn.Validate(order.Number); err != nil {
		return ctx.JSON(http.StatusUnprocessableEntity, map[string]string{"error": apperrors.ErrInvalidOrder.Error()})
	}

	err = h.retryer.Retry(func() error {
		return h.repo.InsertOrder(ctx.Request().Context(), order)
	})

	if err != nil {
		if errors.Is(err, apperrors.ErrOrderInserted) {
			return ctx.JSON(http.StatusOK, err.Error())
		}
		if errors.Is(err, apperrors.ErrOrderInsertedLogin) {
			return ctx.JSON(http.StatusConflict, map[string]string{"error": err.Error()})
		}
		log.Printf("Failed to add order: %v", err)
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": apperrors.ErrServer.Error()})
	}
	return ctx.JSON(http.StatusAccepted, "order accepted")
}

// GetOrders TODO: Пагинация
func (h *userHandler) GetOrders(ctx echo.Context) error {
	userLogin := ctx.Get("user_login").(string)
	var orders []models.OrderResponse

	err := h.retryer.Retry(func() error {
		var err error
		orders, err = h.repo.SelectOrders(ctx.Request().Context(), userLogin)
		return err
	})
	if err != nil {
		log.Printf("Failed to get orders: %v", err)
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": apperrors.ErrServer.Error()})
	}

	if len(orders) < 1 {
		return ctx.JSON(http.StatusNoContent, map[string]string{"error": apperrors.ErrNoData.Error()})
	}

	return ctx.JSON(http.StatusOK, orders)

}

func (h *userHandler) GetBalance(ctx echo.Context) error {
	userLogin := ctx.Get("user_login").(string)
	var balance models.Balance
	err := h.retryer.Retry(func() error {
		var err error
		balance, err = h.repo.SelectBalance(ctx.Request().Context(), userLogin)
		return err
	})
	if err != nil {
		log.Printf("Failed to get balance: %v", err)
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": apperrors.ErrServer.Error()})
	}
	return ctx.JSON(http.StatusOK, balance)
}

func (h *userHandler) Withdraw(ctx echo.Context) error {
	var withdrawal models.Withdrawal
	err := ctx.Bind(&withdrawal)
	if err != nil || withdrawal.Sum <= 0 {
		return ctx.JSON(http.StatusBadRequest, map[string]string{"error": apperrors.ErrInvalidJSON.Error()})
	}

	if err = goluhn.Validate(withdrawal.Order); err != nil {
		return ctx.JSON(http.StatusUnprocessableEntity, map[string]string{"error": apperrors.ErrInvalidOrder.Error()})
	}

	withdrawal.Login = ctx.Get("user_login").(string)
	withdrawal.ProcessedAt = time.Now()

	err = h.retryer.Retry(func() error {
		return h.repo.WithdrawBalance(ctx.Request().Context(), withdrawal)
	})

	if err != nil {
		if errors.Is(err, apperrors.ErrNotEnoughFunds) {
			return ctx.JSON(http.StatusPaymentRequired, map[string]string{"error": err.Error()})
		}
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": apperrors.ErrServer.Error()})
	}
	return ctx.JSON(http.StatusOK, "withdraw successfully")
}

// GetWithdrawals TODO: Пагинация
func (h *userHandler) GetWithdrawals(ctx echo.Context) error {
	userLogin := ctx.Get("user_login").(string)
	var withdrawals []models.WithdrawalResponse

	err := h.retryer.Retry(func() error {
		var err error
		withdrawals, err = h.repo.SelectWithdrawals(ctx.Request().Context(), userLogin)
		return err
	})
	if err != nil {
		log.Printf("Failed to get orders: %v", err)
		return ctx.JSON(http.StatusInternalServerError, map[string]string{"error": apperrors.ErrServer.Error()})
	}

	if len(withdrawals) < 1 {
		return ctx.JSON(http.StatusNoContent, map[string]string{"error": apperrors.ErrNoData.Error()})
	}

	return ctx.JSON(http.StatusOK, withdrawals)

}
