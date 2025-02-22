package orders

import (
	"context"
	"errors"
	"fmt"
	"github.com/golang/mock/gomock"
	"github.com/llaxzi/gophermart/internal/mocks"
	"github.com/llaxzi/gophermart/internal/models"
	"github.com/llaxzi/retryables/v2"
	"github.com/stretchr/testify/assert"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
)

func TestGetNewOrders(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockRepository(ctrl)
	retryer := retryables.NewRetryer(os.Stdout)
	retryer.SetCount(1)

	ordersCh := make(chan models.Order, 10)
	returnedOrdersCh := make(chan models.Order, 10)
	errCh := make(chan error, 1)

	p := processor{
		repo:             repo,
		retryer:          retryer,
		ordersCh:         ordersCh,
		returnedOrdersCh: returnedOrdersCh,
		errCh:            errCh,
		getInterval:      time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	orders := []models.Order{
		{Number: "7458", Login: "test1", Status: "NEW", Accrual: nil, UploadedAt: time.Now()},
		{Number: "7459", Login: "test2", Status: "PROCESSING", Accrual: nil, UploadedAt: time.Now()},
	}

	repo.EXPECT().SelectNewOrders(gomock.Any()).
		Return(orders, nil).Times(1) // Первый вызов - возвращаем заказы

	repo.EXPECT().SelectNewOrders(gomock.Any()).
		Return([]models.Order{}, nil).AnyTimes() // Все последующие вызовы - пустой список

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.getNewOrders(ctx)
	}()
	time.Sleep(2 * time.Second)

	// Проверяем case: order <- returnedOrdersCh
	returnedOrder := models.Order{Number: "7457"}
	returnedOrdersCh <- returnedOrder
	time.Sleep(1 * time.Second)

	cancel()
	wg.Wait()

	// Проверяем, что заказы попали в ordersCh
	var receivedOrders []models.Order
	for len(ordersCh) > 0 {
		receivedOrders = append(receivedOrders, <-ordersCh)
	}

	expectedOrders := append(orders, returnedOrder)
	assert.ElementsMatch(t, expectedOrders, receivedOrders)
}

func TestWorker(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockRepository(ctrl)
	retryer := retryables.NewRetryer(nil)
	retryer.SetCount(1)

	// Accrual mock
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orderID := r.URL.Path[len("/api/orders/"):]
		switch orderID {
		case "too_many_requests":
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
		case "no_content":
			w.WriteHeader(http.StatusNoContent)
		case "error":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"Number": "%s", "Status": "PROCESSED"}`, orderID)
		}
	}))
	defer mockServer.Close()

	tests := []struct {
		name           string
		order          models.Order
		expectedReturn bool
		mockRepoUpdate error
	}{
		{"Successful Order Processing", models.Order{Number: "12345", Status: "NEW"}, false, nil},
		{"No Content", models.Order{Number: "no_content", Status: "NEW"}, true, nil},
		{"Internal Server Error", models.Order{Number: "error", Status: "NEW"}, true, nil},
		{"DB Update Error", models.Order{Number: "db_error", Status: "NEW"}, true, errors.New("db error")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			p := &processor{
				repo:             repo,
				retryer:          retryer,
				accrualAddr:      mockServer.URL,
				ordersCh:         make(chan models.Order, 10),
				returnedOrdersCh: make(chan models.Order, 10),
				errCh:            make(chan error, 10),
			}

			if test.mockRepoUpdate != nil || !test.expectedReturn {
				repo.EXPECT().UpdateOrder(gomock.Any(), gomock.Any()).Return(test.mockRepoUpdate).Times(1)
			}

			var wg sync.WaitGroup
			wg.Add(1)
			go func() {
				defer wg.Done()
				p.worker(ctx)
			}()

			p.ordersCh <- test.order

			cancel()
			wg.Wait()

			// Проверяем, попал ли заказ в returnedOrdersCh
			select {
			case returnedOrder := <-p.returnedOrdersCh:
				if !test.expectedReturn {
					t.Fatalf("Order %s should NOT be returned", returnedOrder.Number)
				}
				assert.Equal(t, test.order.Number, returnedOrder.Number)
			default:
				if test.expectedReturn {
					t.Fatalf("Order %s should be returned but was not", test.order.Number)
				}
			}

			close(p.ordersCh)
			close(p.returnedOrdersCh)
			close(p.errCh)
		})
	}
}

func TestWorker_RetryAfter(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockRepository(ctrl)
	retryer := retryables.NewRetryer(nil)
	retryer.SetCount(1)

	// Accrual mock сначала вернет 429, а потом 200
	requestCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requestCount == 0 {
			w.Header().Set("Retry-After", "2")
			w.WriteHeader(http.StatusTooManyRequests)
			requestCount++
		} else {
			w.WriteHeader(http.StatusOK)
			fmt.Fprintf(w, `{"Number": "retry_after_test", "Status": "PROCESSED"}`)
		}
	}))
	defer mockServer.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p := &processor{
		repo:             repo,
		retryer:          retryer,
		accrualAddr:      mockServer.URL,
		ordersCh:         make(chan models.Order, 10),
		returnedOrdersCh: make(chan models.Order, 10),
		errCh:            make(chan error, 10),
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.worker(ctx)
	}()

	order := models.Order{Number: "retry_after_test", Status: "NEW"}
	p.ordersCh <- order

	start := time.Now()

	var returnedOrder models.Order
	select {
	case returnedOrder = <-p.returnedOrdersCh:
		assert.Equal(t, order.Number, returnedOrder.Number)
	case <-time.After(1 * time.Second): // Если заказ не вернулся за 1 секунду, тест падает
		t.Fatalf("Order %s should be returned immediately after 429 but was not", order.Number)
	}

	// Ожидаем, что UpdateOrder будет вызван после успешной обработки заказа
	repo.EXPECT().UpdateOrder(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	p.ordersCh <- returnedOrder

	// Проверяем, что заказ обработан спустя `Retry-After`
	select {
	case finalOrder := <-p.returnedOrdersCh:
		t.Fatalf("Order %s should not be in returnedOrdersCh again, it should be processed", finalOrder.Number)
	case <-time.After(2 * time.Second):
		duration := time.Since(start)
		assert.GreaterOrEqual(t, duration.Seconds(), 2.0, "Worker did not wait for Retry-After duration")
	}

	cancel()
	wg.Wait()

	close(p.ordersCh)
	close(p.returnedOrdersCh)
	close(p.errCh)
}
