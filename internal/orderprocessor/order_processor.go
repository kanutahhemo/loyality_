package orderprocessor

import (
	"encoding/json"
	"fmt"
	"github.com/hashicorp/go-retryablehttp"
	"github.com/kanutahhemo/loyality_/internal/storage/database"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
	"net/http"
	"time"

	"context"
)

const (
	processingDelay       = 500 * time.Millisecond
	maxConcurrentRequests = 5 // Максимальное количество одновременных запросов
)

type OrderStatus struct {
	OrderNumber int
	Status      string
	Accrual     float64
}

type OrderProcessor struct {
	DB                   database.PgDB
	Logger               *logrus.Logger
	AccrualSystemAddress string
	Cooldown             time.Duration
}

func NewOrderProcessor(db database.PgDB, logger *logrus.Logger, address string, cooldown time.Duration) *OrderProcessor {
	return &OrderProcessor{
		DB:                   db,
		Logger:               logger,
		AccrualSystemAddress: address,
		Cooldown:             cooldown,
	}
}

func (op *OrderProcessor) Start() {
	for {
		op.processOrdersConcurrently()
		time.Sleep(processingDelay)
	}
}

func (op *OrderProcessor) processOrdersConcurrently() {
	numbers, err := op.DB.GetActiveOrders()

	if err != nil {
		op.Logger.Errorf("Error getting active orders: %s", err)
		return
	}

	sem := semaphore.NewWeighted(int64(maxConcurrentRequests))
	eg := errgroup.Group{}

	for _, number := range numbers {
		currentNumber := number
		sem.Acquire(context.TODO(), 1)
		eg.Go(func() error {
			defer sem.Release(1)
			orderStatus, err := op.getOrderStatusFromAccrualSystem(currentNumber)
			if err != nil {
				op.Logger.Errorf("Error getting order status for order %s: %s", currentNumber, err)
				return err
			}

			// Обработка статуса заказа
			switch orderStatus.Status {
			case "PROCESSED":

				op.updateOrderStatus(currentNumber, "processed", orderStatus.Accrual)
			case "REGISTERED", "PROCESSING":
				op.updateOrderStatus(currentNumber, "processing", 0)
				time.Sleep(processingDelay)
				// Оставляем пустой case для "INVALID", так как вам нужно выполнить определенные действия
			default:
				op.Logger.Printf("Unknown order status: %s", orderStatus.Status)
			}
			return nil
		})
	}
}

func (op *OrderProcessor) getOrderStatusFromAccrualSystem(orderNumber string) (*OrderStatus, error) {
	retryClient := retryablehttp.NewClient()
	retryClient.RetryMax = 3
	retryClient.Backoff = retryablehttp.LinearJitterBackoff

	url := fmt.Sprintf("%s/api/orders/%s", op.AccrualSystemAddress, orderNumber)

	response, err := retryClient.Get(url)
	if err != nil {
		op.Logger.Errorf("Error sending request: %s", err)
		return nil, err
	}

	defer response.Body.Close()

	if response.StatusCode == http.StatusTooManyRequests {
		op.Logger.Errorf("Received status 429 - Too Many Requests. Retrying after 2 seconds...")
		time.Sleep(time.Second * 2)
		return op.getOrderStatusFromAccrualSystem(orderNumber) // Повторный запрос после таймаута
	} else if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-OK status code: %d", response.StatusCode)
	}

	var orderStatus OrderStatus
	err = json.NewDecoder(response.Body).Decode(&orderStatus)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	return &orderStatus, nil
}

func (op *OrderProcessor) updateOrderStatus(orderNumber string, status string, accrual float64) {
	fmt.Println(orderNumber, status, accrual)
	err := op.DB.UpdateOrderStatus(orderNumber, status, accrual)
	if err != nil {

		op.Logger.Errorf("order_processor %s", err)
	}
	op.Logger.Printf("Order %s updated: Status=%s, Accrual=%.2f", orderNumber, status, accrual)
}
