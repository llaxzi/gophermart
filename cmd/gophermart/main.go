package main

import (
	"context"
	"errors"
	"github.com/labstack/echo/v4"
	"github.com/llaxzi/gophermart/internal/apperrors"
	"github.com/llaxzi/gophermart/internal/handler"
	"github.com/llaxzi/gophermart/internal/middleware"
	"github.com/llaxzi/gophermart/internal/orders"
	"github.com/llaxzi/gophermart/internal/repository"
	"github.com/llaxzi/gophermart/internal/tokens"
	"github.com/llaxzi/retryables/v2"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {

	// Получаем env переменные и флаги
	parseVars()

	// Временно выставляем вручную
	/*databaseDSN = "postgres://dev:qwerty@localhost:5433/gophermart?sslmode=disable"
	runAddr = ":8081"
	accrualAddr = "http://localhost:8080"*/

	repo, err := repository.NewRepository(databaseDSN)
	if err != nil {
		log.Fatalf("Failed to initialize repository: %v", err)
		return
	}

	retryer := retryables.NewRetryer(os.Stdout)
	retryer.SetCount(3)
	retryer.SetDelay(1, 2)
	retryer.SetConditionFunc(func(err error) bool {
		return errors.Is(err, apperrors.ErrPgConnExc)
	})

	// Запуск миграции из сервиса, при необходимости
	err = repo.Bootstrap(databaseDSN, 1)
	if err != nil {
		log.Fatalf("Failed to bootstrap repository: %v", err)
		return
	}

	tokenB := tokens.NewTokenBuilder([]byte("key"), time.Hour*3)

	mid := middleware.NewMiddleware([]byte("key"))

	userHandler := handler.NewUserHandler(repo, tokenB, retryer)

	e := echo.New()

	e.POST("/api/user/register", userHandler.Register)
	e.POST("/api/user/login", userHandler.Login)

	auth := e.Group("", mid.Auth)
	gzip := auth.Group("", mid.Gzip)

	auth.POST("/api/user/orders", userHandler.AddOrder)
	gzip.GET("/api/user/orders", userHandler.GetOrders)
	auth.GET("/api/user/balance", userHandler.GetBalance)
	auth.POST("/api/user/balance/withdraw", userHandler.Withdraw)
	gzip.GET("/api/user/withdrawals", userHandler.GetWithdrawals)

	processor := orders.NewProcessor(repo, retryer, accrualAddr, 1*time.Second, 5)
	ctx, cancel := context.WithCancel(context.Background())
	go processor.ProcessOrders(ctx)

	// Запускаем сервер
	go func() {
		if err = e.Start(runAddr); err != nil {
			log.Printf("Shutting down server: %v", err)
			cancel()
		}
	}()

	// Перехватываем сигнал Ctrl+C
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)

	<-signalCh
	cancel()
}
