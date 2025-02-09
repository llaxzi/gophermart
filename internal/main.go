package main

import (
	"context"
	"errors"
	"fmt"
	"github.com/llaxzi/gophermart/internal/apperrors"
	"github.com/llaxzi/gophermart/internal/models"
	"github.com/llaxzi/gophermart/internal/repository"
	"github.com/llaxzi/retryables"
	"log"
)

func main() {
	databaseDSN := "postgres://dev:qwerty@localhost:5433/gophermart?sslmode=disable"
	repo, err := repository.NewRepository(databaseDSN)
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

	i := float64(20)
	order := models.Order{
		Number:  "12345678903",
		Login:   "test",
		Status:  "PROCESSED",
		Accrual: &i,
	}

	err = retryer.Retry(func() error {
		return repo.UpdateOrder(context.TODO(), order)
	})
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("updated")

}
