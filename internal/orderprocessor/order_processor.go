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
	processingDelay = 1 * time.Second
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

func (op *OrderProcessor) Start() {
	for {
		op.processOrders()
		time.Sleep(processingDelay)
	}
}

func (op *OrderProcessor) processOrders() {
	numbers, err := op.DB.GetActiveOrders()

	if err != nil {
		op.Logger.Errorf("Error getting active orders: %s", err)
		return
	}

	for _, number := range numbers {
		orderStatus, err := op.getOrderStatusFromAccrualSystem(number)
		if err != nil {
			op.Logger.Errorf("Error getting order status for order %s: %s", number, err)
			continue
		}

		// Обработка статуса заказа
		switch orderStatus.Status {
		case "PROCESSED":

			op.updateOrderStatus(number, "processed", orderStatus.Accrual)
		case "REGISTERED", "PROCESSING":
			op.updateOrderStatus(number, "processing", 0)
			time.Sleep(processingDelay)
			// Оставляем пустой case для "INVALID", так как вам нужно выполнить определенные действия
		default:
			op.Logger.Printf("Unknown order status: %s", orderStatus.Status)
		}
	}
}

func (op *OrderProcessor) getOrderStatusFromAccrualSystem(orderNumber string) (*OrderStatus, error) {
	url := fmt.Sprintf("%s/api/orders/%s", op.AccrualSystemAddress, orderNumber)

	client := http.Client{
		Timeout: time.Second * 10, // Таймаут для запроса
	}

	response, err := client.Get(url)
	if err != nil {
		fmt.Println(err)
		return nil, err
	}

	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {

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
