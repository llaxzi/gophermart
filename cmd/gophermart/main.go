package main

import (
	"errors"
	"github.com/labstack/echo/v4"
	"github.com/llaxzi/gophermart/internal/apperrors"
	"github.com/llaxzi/gophermart/internal/handler"
	"github.com/llaxzi/gophermart/internal/middleware"
	"github.com/llaxzi/gophermart/internal/repository"
	"github.com/llaxzi/gophermart/internal/tokens"
	"github.com/llaxzi/retryables"
	"log"
	"time"
)

func main() {

	repo, err := repository.NewRepository("postgres://dev:qwerty@localhost:5433/gophermart?sslmode=disable")
	if err != nil {
		log.Fatalf("Failed to initialize repository: %v", err)
		return
	}

	retryer := retryables.NewRetryer()
	retryer.SetCount(3)
	retryer.SetDelay(1, 2)
	retryer.SetConditionFunc(func(err error) bool {
		return errors.Is(err, apperrors.ErrPgConnExc)
	})

	/*err = repo.Bootstrap("postgres://dev:qwerty@localhost:5433/gophermart?sslmode=disable", 1)
	if err != nil {
		log.Fatalf("Failed to bootstrap repository: %v", err)
		return
	}*/

	tokenB := tokens.NewTokenBuilder([]byte("key"), time.Hour*3)

	mid := middleware.NewMiddleware([]byte("key"))

	userHandler := handler.NewUserHandler(repo, tokenB, retryer)

	e := echo.New()

	e.POST("/user/register", userHandler.Register)
	e.POST("/user/login", userHandler.Login)

	auth := e.Group("", mid.Auth)
	gzip := auth.Group("", mid.Gzip)

	auth.POST("/user/orders", userHandler.AddOrder)
	gzip.GET("/user/orders", userHandler.GetOrders)
	auth.GET("/user/balance", userHandler.GetBalance)

	e.Logger.Fatal(e.Start(":8081"))
}
