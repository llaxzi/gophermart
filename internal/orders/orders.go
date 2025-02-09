package orders

import (
	"context"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/llaxzi/gophermart/internal/models"
	"github.com/llaxzi/gophermart/internal/repository"
	"github.com/llaxzi/retryables"
	"log"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

type Processor interface {
	ProcessOrders(ctx context.Context)
}

func NewProcessor(repo repository.Repository, retryer retryables.Retryer, accrualAddr string,
	getInterval int, workerCount int) Processor {
	p := &processor{
		repo:             repo,
		retryer:          retryer,
		accrualAddr:      accrualAddr,
		ordersCh:         make(chan models.Order, 50),
		returnedOrdersCh: make(chan models.Order, 50),
		errCh:            make(chan error, 50),
		getInterval:      getInterval,
		workerCount:      workerCount,
	}
	return p
}

type processor struct {
	repo             repository.Repository
	retryer          retryables.Retryer
	accrualAddr      string
	ordersCh         chan models.Order
	returnedOrdersCh chan models.Order
	errCh            chan error
	retryAfter       atomic.Value
	getInterval      int
	workerCount      int
}

// GetNewOrders - generator
func (p *processor) getNewOrders(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(p.getInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			var orders []models.Order
			err := p.retryer.Retry(func() error {
				var err error
				orders, err = p.repo.SelectNewOrders(ctx)
				return err
			})
			if err != nil {
				p.errCh <- fmt.Errorf("failed to select new orders: %w", err)
				continue
			}

			go func(orders []models.Order) {
				for _, order := range orders {
					select {
					case <-ctx.Done():
						return
					case p.ordersCh <- order:
					}
				}
			}(orders)
		case order := <-p.returnedOrdersCh:
			go func(order models.Order) {
				select {
				case <-ctx.Done():
					return
				case p.ordersCh <- order:
				}
			}(order)
		}
	}
}

func (p *processor) ProcessOrders(ctx context.Context) {
	go p.getNewOrders(ctx)

	var wg sync.WaitGroup
	// Запускаем воркеры
	for i := 0; i < p.workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			p.worker(ctx)
		}()
	}

	// Обработка ошибок
	go func() {
		for {
			select {
			case err := <-p.errCh:
				log.Printf(err.Error())
			case <-ctx.Done():
				return
			}
		}
	}()

	<-ctx.Done()

	wg.Wait()

	// Возвращаем необработанные заказы
	close(p.ordersCh)
	for order := range p.ordersCh {
		p.resetOrderStatus(ctx, order.Number)
	}
	close(p.returnedOrdersCh)
	for order := range p.returnedOrdersCh {
		p.resetOrderStatus(ctx, order.Number)
	}

	close(p.errCh)

}

// TODO: потенциальная самоблокировка
func (p *processor) worker(ctx context.Context) {
	client := resty.New()
	// Настройка retry
	client.SetRetryCount(3)
	client.SetRetryAfter(func(client *resty.Client, response *resty.Response) (time.Duration, error) {
		retryCount := response.Request.Attempt
		switch retryCount {
		case 1:
			return 1 * time.Second, nil
		case 2:
			return 3 * time.Second, nil
		case 3:
			return 5 * time.Second, nil
		default:
			return 0, nil
		}
	})
	client.AddRetryCondition(func(response *resty.Response, err error) bool {
		if response.StatusCode() == http.StatusServiceUnavailable || response.StatusCode() == http.StatusInternalServerError {
			return true
		}
		return false
	}) // retry только в случае, если сервер недоступен (maintenance или перегрузка) или внут. ошибка

	for {
		select {
		case order := <-p.ordersCh:
			// Ждём, если нужно
			p.wait()
			fmt.Println("Worker skipped wait()")

			resp, err := client.R().SetResult(&order).Get(p.accrualAddr + "/api/orders/" + order.Number)

			fmt.Printf("Response status: %d, error: %v\n", resp.StatusCode(), err)
			fmt.Printf("Body: %v\n", resp.Body())
			fmt.Printf("Order after response: %v\n", order)

			if err != nil {
				p.errCh <- fmt.Errorf("failed to send request: %w", err)
				p.returnedOrdersCh <- order
				continue
			}

			if resp.StatusCode() == http.StatusTooManyRequests {
				retryAfterStr := resp.Header().Get("Retry-After")
				retryAfter, err := strconv.Atoi(retryAfterStr)
				if err != nil {
					p.errCh <- fmt.Errorf("failed to atoi: %w", err)
					p.returnedOrdersCh <- order
					fmt.Printf("Failed to atoi: %v\n", err)
					continue
				}
				if retryAfter > 0 {
					p.setDelay(time.Now().Add(time.Duration(retryAfter) * time.Second))
				}
				p.returnedOrdersCh <- order
				fmt.Printf("Got Retry-After: %v\n", retryAfter)
				continue
			}

			if resp.StatusCode() == http.StatusNoContent {
				fmt.Printf("Skipping order: %v - No content\n", order)
				p.resetOrderStatus(ctx, order.Number)
				continue
			}

			if order.Status == "PROCESSING" || order.Status == "REGISTERED" {
				p.ordersCh <- order
				continue
			}

			fmt.Printf("Updating order in DB: Number=%s, Status=%s, Accrual=%v\n", order.Number, order.Status, *order.Accrual)
			err = p.retryer.Retry(func() error {
				return p.repo.UpdateOrder(ctx, order)
			})
			if err != nil {
				p.errCh <- fmt.Errorf("failed to update order: %w", err)
				p.returnedOrdersCh <- order
				fmt.Printf("Failed to update order: %v\n", err)
				continue
			}
			fmt.Printf("Updated order: %v\n", order)
		case <-ctx.Done():
			return
		}
	}
}

func (p *processor) setDelay(after time.Time) {
	p.retryAfter.Store(after)
}

func (p *processor) wait() {
	if after, ok := p.retryAfter.Load().(time.Time); ok && time.Now().Before(after) {
		time.Sleep(time.Until(after))
	}
}

func (p *processor) resetOrderStatus(ctx context.Context, orderNumber string) {
	err := p.retryer.Retry(func() error {
		return p.repo.ResetStatus(ctx, orderNumber)
	})
	if err != nil {
		p.errCh <- fmt.Errorf("failed to reset order status: %w", err)
		fmt.Printf("Reset order status for order: %v error: %v\n", orderNumber, err)
	}
	fmt.Printf("Reset order status for order: %v\n", orderNumber)
}
