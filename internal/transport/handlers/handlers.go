package handlers

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/go-chi/chi/v5"
	"github.com/kanutahhemo/loyality_/internal/config"
	"github.com/kanutahhemo/loyality_/internal/storage/database"
	"github.com/kanutahhemo/loyality_/internal/storage/encryption"
	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/bcrypt"
	"io"
	"math/big"
	"net/http"
	"sort"
	"strconv"
	"time"
	"unicode"
)

type msgJSON struct {
	MsgStr string `json:"MsgStr"`
	ErrStr string `json:"ErrStr"`
}

func isLuhnValid(number string) bool {
	runes := []rune(number)
	sum := 0
	alt := false

	for i := len(runes) - 1; i >= 0; i-- {
		if !unicode.IsDigit(runes[i]) {
			return false
		}

		digit := int(runes[i]) - '0'

		if alt {
			digit = digit * 2
			if digit > 9 {
				digit = digit - 9
			}
		}

		sum = sum + digit
		alt = !alt
	}

	return sum%10 == 0
}

func generateRandomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	charsetLength := big.NewInt(int64(len(charset)))

	bytes := make([]byte, length)
	for i := 0; i < length; i++ {
		index, err := rand.Int(rand.Reader, charsetLength)
		if err != nil {
			return "", err
		}
		bytes[i] = charset[index.Int64()]
	}

	return string(bytes), nil
}

func ExtractTokenFromRequest(req *http.Request) (string, error) {

	authHeader := req.Header.Get("Authorization")
	if authHeader == "" {
		return "", http.ErrNoCookie
	}

	tokenStr := authHeader[len("Bearer "):]
	return tokenStr, nil
}

type Claims struct {
	UserID int `json:"user_id"`
	jwt.StandardClaims
}

func GenerateToken(userID int, secretKey []byte) (string, error) {
	claims := Claims{
		UserID: userID,
		StandardClaims: jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Hour * 1).Unix(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(secretKey)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return tokenString, nil
}

func SetTokenInResponse(w http.ResponseWriter, tokenString string) {

	w.Header().Set("Authorization", "Bearer "+tokenString)
}

func AuthMiddleware(logger *logrus.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			logger.Debug("AuthMiddleware start")

			tokenStr, err := ExtractTokenFromRequest(req)
			if err != nil {
				logger.Errorf("AuthMiddleware : %s", err)
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(msgJSON{ErrStr: "Unauthorized"})
				return
			}

			token, err := jwt.Parse(tokenStr, func(token *jwt.Token) (interface{}, error) {
				logger.Errorf("AuthMiddleware : %s", err)
				return []byte(config.Cfg.SecretKey), nil
			})
			if err != nil || !token.Valid {
				logger.Errorf("AuthMiddleware : %s", err)
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(msgJSON{ErrStr: "Unauthorized"})
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				logger.Errorf("AuthMiddleware : failed to get user information from token")
				w.WriteHeader(http.StatusInternalServerError)
				json.NewEncoder(w).Encode(msgJSON{ErrStr: "Failed to get user information from token"})
				return
			}

			userID := int(claims["user_id"].(float64))

			ctx := context.WithValue(req.Context(), "userID", userID)

			next.ServeHTTP(w, req.WithContext(ctx))
			logger.Debug("AuthMiddleware end")
		})
	}
}

func Ping(db database.PgDB) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		_, err := db.Ping()
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("PONG"))
	}
}

func UserRegister(db database.PgDB, logger *logrus.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		logger.Debug("UserRegister handler start")
		// Проверка Content-Type
		if req.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			logger.Errorf("UserRegister Handler : Wrong Content Type")
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Wrong Content Type"})
			return
		}
		w.Header().Set("Content-Type", "application/json")

		// Чтение тела запроса
		bodyJSON, err := io.ReadAll(req.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			logger.Errorf("UserRegister Handler : %s", err)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Can't parse request body"})
			return
		}
		user := database.User{}
		err = json.Unmarshal(bodyJSON, &user)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			logger.Errorf("UserRegister Handler : %s", err)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Can't parse JSON content"})
			return
		}

		// Хеширование пароля
		user.Password, err = encryption.HashPassword(user.Password)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			logger.Errorf("UserRegister Handler : %s", err)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Error with making hash of the password"})
			return
		}

		// Регистрация пользователя
		uid, err := db.UserRegister(user)
		if err != nil {
			if err.Error() == "not unique user" {
				w.WriteHeader(http.StatusConflict)
				logger.Errorf("UserRegister Handler : %s", err)
				json.NewEncoder(w).Encode(msgJSON{
					MsgStr: "This login is already exists",
					ErrStr: "not unique user",
				})
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			logger.Errorf("UserRegister Handler : %s", err)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Can't register user"})
			return
		}

		tokenString, err := GenerateToken(uid, []byte(config.Cfg.SecretKey))
		if err != nil {
			logger.Fatal("error with generating token")
		}

		SetTokenInResponse(w, tokenString)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(msgJSON{MsgStr: "Registration Successful"})
		logger.Debug("UserRegister handler stop")
	}
}

func UserLogin(db database.PgDB, logger *logrus.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		logger.Debug("UserLogin handler start")
		// Проверка Content-Type
		if req.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Wrong Content Type"})
			return
		}
		w.Header().Set("Content-Type", "application/json")

		// Чтение тела запроса
		bodyJSON, err := io.ReadAll(req.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			logger.Errorf("UserLogin Handler : %s", err)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Can't parse request body"})
			return
		}
		user := database.User{}
		err = json.Unmarshal(bodyJSON, &user)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			logger.Errorf("UserLogin Handler : %s", err)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Can't parse JSON content"})
			return
		}

		pwHash, uid, err := db.UserLogin(user)
		if err != nil {
			logger.Errorf("UserLogin Handler : %s", err)
			if err.Error() == "invalid login/password pair" {
				w.WriteHeader(http.StatusUnauthorized)
				json.NewEncoder(w).Encode(msgJSON{ErrStr: "Invalid login/password pair"})
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Something goes wrong"})
			return
		}

		err = bcrypt.CompareHashAndPassword([]byte(pwHash), []byte(user.Password))
		if err != nil {
			logger.Errorf("UserLogin Handler : %s", err)
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Invalid login/password pair"})
			return
		}

		tokenString, err := GenerateToken(uid, []byte(config.Cfg.SecretKey))
		if err != nil {
			logger.Fatalf("UserLogin Handler : %s", err)
		}

		SetTokenInResponse(w, tokenString)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(msgJSON{MsgStr: "Login Successful"})
		logger.Debug("UserLogin handler end")
	}

}

func UserAddOrder(db database.PgDB, logger *logrus.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		logger.Debug("UserAddOrder handler start")
		userID, ok := req.Context().Value("userID").(int)
		if !ok {
			logger.Errorf("UserAddOrder Handler : Failed to get userID from request context")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Failed to get userID from request context"})
			return
		}

		if req.Header.Get("Content-Type") != "text/plain" {
			logger.Errorf("UserAddOrder Handler : Wrong Content Type")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Wrong Content Type"})
			return
		}

		w.Header().Set("Content-Type", "application/json")

		bodyText, err := io.ReadAll(req.Body)
		if err != nil {
			logger.Errorf("UserAddOrder Handler : %s", err)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Can't parse request body"})
			return
		}

		if !isLuhnValid(string(bodyText)) {
			logger.Errorf("UserAddOrder Handler : Wrong order format")
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Wrong order format"})
			return
		}

		bodyNumber, err := strconv.Atoi(string(bodyText))
		if err != nil {
			logger.Errorf("UserAddOrder Handler : %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: err.Error()})
			return
		}

		_, err = db.UserAddOrder(userID, bodyNumber)
		if err != nil {
			logger.Errorf("UserAddOrder Handler : %s", err)
			if err.Error() == "not unique order" {
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(msgJSON{
					MsgStr: "This order is already exists",
					ErrStr: err.Error(),
				})
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: err.Error()})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(msgJSON{MsgStr: "User Added Order Successfully"})
		logger.Debug("UserAddOrder handler end")
	}
}

func UserOrders(db database.PgDB, logger *logrus.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		logger.Debug("UserOrders handler start")
		userID, ok := req.Context().Value("userID").(int)
		if !ok {
			logger.Errorf("UserOrders Handler : Failed to get userID from request context")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Failed to get userID from request context"})
			return
		}

		orders, err := db.UserOrders(userID)
		if err != nil {
			logger.Errorf("UserOrders Handler : %s", err)
			fmt.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		sort.Slice(orders, func(i, j int) bool {
			return orders[i].UploadedAt.Before(orders[j].UploadedAt)
		})

		if len(orders) == 0 {
			logger.Infof("UserOrders Handler : no orders")
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "no orders"})
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		err = json.NewEncoder(w).Encode(orders)
		if err != nil {
			logger.Errorf("UserOrders Handler : %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		logger.Debug("UserOrders handler end")
	}
}

func UserBalance(db database.PgDB, logger *logrus.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		logger.Debug("UserBalance handler start")
		userID, ok := req.Context().Value("userID").(int)
		if !ok {
			logger.Errorf("UserBalance Handler : Failed to get userID from request context")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Failed to get userID from request context"})
			return
		}

		current, withdrawn, err := db.UserBalance(userID)
		if err != nil {
			logger.Errorf("UserBalance Handler : %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		response := struct {
			Current   float64 `json:"current"`
			Withdrawn float64 `json:"withdrawn"`
		}{
			Current:   current,
			Withdrawn: withdrawn,
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		err = json.NewEncoder(w).Encode(response)
		if err != nil {
			logger.Errorf("UserBalance Handler : %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		logger.Debug("UserBalance handler end")
	}
}

func UserBalanceWithdraw(db database.PgDB, logger *logrus.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		logger.Debug("UserBalanceWithdraw handler start")
		userID, ok := req.Context().Value("userID").(int)
		if !ok {
			logger.Errorf("UserBalanceWithdraw Handler :Failed to get userID from request context")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Failed to get userID from request context"})
			return
		}

		bodyText, err := io.ReadAll(req.Body)
		if err != nil {
			logger.Errorf("UserBalanceWithdraw Handler : %s", err)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: err.Error()})
			return
		}

		var data database.Withdrawal
		err = json.Unmarshal(bodyText, &data)
		if err != nil {
			logger.Errorf("UserBalanceWithdraw Handler : %s", err)
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: err.Error()})
			return
		}

		if !isLuhnValid(data.Order) {
			logger.Errorf("UserBalanceWithdraw Handler : %s", err)
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Wrong order format"})
			return
		}

		result, err := db.UserBalanceWithdraw(userID, data.Sum, data.Order)
		if err != nil {
			logger.Errorf("UserBalanceWithdraw Handler : %s", err)
			fmt.Println(err.Error())
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if !result {

			w.WriteHeader(http.StatusPaymentRequired)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		w.WriteHeader(http.StatusOK)

		err = json.NewEncoder(w).Encode(msgJSON{
			MsgStr: "Successfully withdrawn",
			ErrStr: "",
		})

		if err != nil {
			logger.Errorf("UserBalanceWithdraw Handler : %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		logger.Debug("UserBalanceWithdraw handler end")
	}
}

func UserWithdrawals(db database.PgDB, logger *logrus.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		logger.Debug("UserWithdrawals handler start")
		userID, ok := req.Context().Value("userID").(int)
		if !ok {
			logger.Errorf("UserWithdrawals Handler : Failed to get userID from request context")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Failed to get userID from request context"})
			return
		}

		var result []database.Withdrawal

		result, err := db.UserWithdrawals(userID)
		if err != nil {
			logger.Errorf("UserWithdrawals Handler : %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: err.Error()})
			return
		}

		if len(result) == 0 {
			logger.Infof("UserWithdrawals Handler : no withdrawals")
			w.WriteHeader(http.StatusNoContent)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "no withdrawals"})
			return
		}

		w.Header().Set("Content-Type", "application/json")

		w.WriteHeader(http.StatusOK)

		err = json.NewEncoder(w).Encode(result)
		if err != nil {
			logger.Errorf("UserWithdrawals Handler : %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Println(err.Error())
			return
		}
		logger.Debug("UserWithdrawals handler end")
	}
}

func UserGetOrder(db database.PgDB, logger *logrus.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		logger.Debug("UserGetOrder handler start")
		userID, ok := req.Context().Value("userID").(int)
		if !ok {
			logger.Errorf("UserGetOrder Handler : Failed to get userID from request context")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: "Failed to get userID from request context"})
			return
		}

		orderIDStr := chi.URLParam(req, "number")
		orderID, err := strconv.Atoi(orderIDStr)
		if err != nil {
			logger.Errorf("UserGetOrder Handler : %s", err)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: err.Error()})
			return
		}

		var result database.Order
		var limit int = 3 //TODO configure limit from args
		result, err = db.UserGetOrder(userID, orderID, limit)
		if err != nil {
			logger.Errorf("UserGetOrder Handler : %s", err)
			if err.Error() == "Too Many Requests" {
				w.WriteHeader(http.StatusTooManyRequests)
				message := fmt.Sprintf("No more than %v requests per minute allowed", limit)
				w.Write([]byte(message))
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(msgJSON{ErrStr: err.Error()})
			return
		}

		w.Header().Set("Content-Type", "application/json")

		w.WriteHeader(http.StatusOK)

		err = json.NewEncoder(w).Encode(result)
		if err != nil {
			logger.Errorf("UserGetOrder Handler : %s", err)
			w.WriteHeader(http.StatusInternalServerError)
			fmt.Println(err.Error())
			return
		}
		logger.Debug("UserGetOrder handler end")
	}
}
