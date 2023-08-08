package orderprocessor

import (
	"encoding/json"
	"fmt"
	"github.com/kanutahhemo/loyality_/internal/storage/database"
	"github.com/sirupsen/logrus"
	"net/http"
	"time"
)

const (
	PROCESSING_DELAY = 200 * time.Millisecond
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
}

func NewOrderProcessor(db database.PgDB, logger *logrus.Logger, address string) *OrderProcessor {
	return &OrderProcessor{
		DB:                   db,
		Logger:               logger,
		AccrualSystemAddress: address,
	}
}

func (op *OrderProcessor) Start(orderChannel chan int) {

	go op.processOrders()

}

func (op *OrderProcessor) processOrders() {
	numbers, err := op.DB.GetActiveOrders()
	if err != nil {
		op.Logger.Errorf("Error getting active orders: %s", err)
		return
	}

	for _, number := range numbers {
		orderStatus, err := op.getOrderStatusFromAccrualSystem(int(number))
		if err != nil {
			op.Logger.Errorf("Error getting order status for order %d: %s", number, err)
			continue
		}

		// Обработка статуса заказа
		switch orderStatus.Status {
		case "PROCESSED":
			op.updateOrderStatus(int64(number), "processed", orderStatus.Accrual)
		case "REGISTERED", "PROCESSING":
			op.updateOrderStatus(int64(number), "processing", 0)
			time.Sleep(PROCESSING_DELAY)
			// Оставляем пустой case для "INVALID", так как вам нужно выполнить определенные действия
		default:
			op.Logger.Printf("Unknown order status: %s", orderStatus.Status)
		}
	}
}

func (op *OrderProcessor) getOrderStatusFromAccrualSystem(orderNumber int) (*OrderStatus, error) {
	url := fmt.Sprintf("%s/api/orders/%d", op.AccrualSystemAddress, orderNumber)

	client := http.Client{
		Timeout: time.Second * 10, // Таймаут для запроса
	}

	response, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-OK status code: %d", response.StatusCode)
	}

	var orderStatus OrderStatus
	err = json.NewDecoder(response.Body).Decode(&orderStatus)
	if err != nil {
		return nil, err
	}

	return &orderStatus, nil
}

func (op *OrderProcessor) updateOrderStatus(orderNumber int64, status string, accrual float64) {

	err := op.DB.UpdateOrderStatus(orderNumber, status, accrual)
	if err != nil {
		op.Logger.Errorf("order_processor %s", err)
	}
	op.Logger.Printf("Order %d updated: Status=%s, Accrual=%.2f", orderNumber, status, accrual)
}
