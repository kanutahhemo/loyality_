package server

import (
	"github.com/go-chi/chi/v5"
	"github.com/kanutahhemo/loyality_/internal/config"
	"github.com/kanutahhemo/loyality_/internal/storage/database"
	"github.com/kanutahhemo/loyality_/internal/transport/handlers"
	"github.com/sirupsen/logrus"
	"net/http"
)

func RunServer(cfg config.Config, pgDB *database.PgDB, logger *logrus.Logger) {
	logger.Debugf("test")
	r := chi.NewRouter()

	r.Get("/ping", handlers.Ping(*pgDB))
	r.Post("/api/user/register", handlers.UserRegister(*pgDB, logger))
	r.Post("/api/user/login", handlers.UserLogin(*pgDB, logger))

	r.Group(func(r chi.Router) {
		r.Use(handlers.AuthMiddleware(logger))
		r.Post("/api/user/orders", handlers.UserAddOrder(*pgDB, logger))
		r.Get("/api/user/orders", handlers.UserOrders(*pgDB, logger))
		r.Get("/api/user/balance", handlers.UserBalance(*pgDB, logger))
		r.Post("/api/user/balance/withdraw", handlers.UserBalanceWithdraw(*pgDB, logger))
		r.Get("/api/user/withdrawals", handlers.UserWithdrawals(*pgDB, logger))
		r.Get("/api/orders/{number}", handlers.UserGetOrder(*pgDB, logger))
	})

	server := &http.Server{
		Addr:    cfg.ServerAddress,
		Handler: r,
	}

	logger.Printf("Server is running on %s", cfg.ServerAddress)
	if err := server.ListenAndServe(); err != nil {
		logger.Fatalf("Server error: %s", err)
	}
}
