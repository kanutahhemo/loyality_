package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"strconv"
	"time"
)

type User struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

type MyPool interface {
	Query(ctx context.Context, sql string, arguments ...interface{}) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, arguments ...interface{}) pgx.Row
	Close()
}

type Withdrawal struct {
	Order string    `json:"order"`
	Sum   float64   `json:"sum"`
	Date  time.Time `json:"date,omitempty"`
}

type Order struct {
	Number     string    `json:"number,omitempty"`
	Order      int       `json:"order,omitempty"`
	Status     string    `json:"status"`
	Accrual    *float64  `json:"accrual,omitempty"`
	UploadedAt time.Time `json:"uploaded_at,omitempty"`
}

type PgDB struct {
	Pool       MyPool
	Context    context.Context
	CancelFunc context.CancelFunc
}

//go:embed migrations/*.sql
var embedMigrations embed.FS //

func ApplyMigrations(dsn string) error {
	fmt.Println(dsn)
	driver, err := iofs.New(embedMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("failed to create migration driver: %w", err)
	}

	m, err := migrate.NewWithSourceInstance("iofs", driver, dsn)
	if err != nil {
		return fmt.Errorf("failed to create migration instance: %w", err)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	return nil
}

func NewPgDatabase(dsn string) (*PgDB, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to parse database connection config: %w", err)
	}

	config.MaxConns = 10 //TODO: make in configurable with arguments
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	db, err := pgxpool.ConnectConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to the database: %w", err)
	}

	pgdb := &PgDB{
		Pool:       db,
		Context:    ctx,
		CancelFunc: cancel,
	}

	return pgdb, nil
}

func (d *PgDB) Close() {
	d.Pool.Close()
}

func (d *PgDB) UserRegister(user User) (int, error) {
	var uid int
	err := d.Pool.QueryRow(context.Background(), `
	INSERT INTO sp_users (login, password) values ($1, $2) returning uid
`, user.Login, user.Password).Scan(&uid)

	if err != nil {
		pgErr, ok := err.(*pgconn.PgError)
		if ok && pgErr.Code == "23505" {
			return 0, fmt.Errorf("not unique user")
		}
		return 0, fmt.Errorf("failed with inserting user to the database")
	}

	return uid, nil
}

func (d *PgDB) UserLogin(user User) (string, int, error) {

	var password string
	var uid int
	result := d.Pool.QueryRow(context.Background(), `
		select uid, password from sp_users where login=$1
    `, user.Login)

	err := result.Scan(&uid, &password)

	if err != nil {

		if err == sql.ErrNoRows {
			return "", 0, fmt.Errorf("invalid login/password pair")
		}
		return "", 0, fmt.Errorf("failed select user from the database")
	}

	return password, uid, nil
}

func (d *PgDB) UserAddOrder(userID int, order string) (int, error) {
	var orderID int
	err := d.Pool.QueryRow(context.Background(), `
	INSERT INTO sp_orders (uid, order_value, status_id) values ($1, $2, (select status_id from sp_statuses where status_value='NEW')) returning order_id
`, userID, order).Scan(&orderID)
	if err != nil {
		pgErr, ok := err.(*pgconn.PgError)
		if ok && pgErr.Code == "23505" {
			return 0, fmt.Errorf("not unique order")
		}
		return 0, fmt.Errorf("failed with inserting order to the database")
	}
	return orderID, nil
}

func (d *PgDB) UserOrders(userID int) ([]Order, error) {

	rows, err := d.Pool.Query(context.Background(), `
		SELECT so.order_value, ss.status_value, so.accrual, so.created_time
		FROM sp_orders so
		INNER JOIN sp_statuses ss ON so.status_id = ss.status_id
		INNER JOIN sp_users su ON so.uid = su.uid
		WHERE su.uid = $1
		ORDER BY so.created_time ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user orders: %w", err)
	}

	var orders []Order

	for rows.Next() {
		var order Order
		err := rows.Scan(&order.Number, &order.Status, &order.Accrual, &order.UploadedAt)

		if err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		orders = append(orders, order)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate over rows: %w", err)
	}

	return orders, nil
}

func (d *PgDB) UserBalance(userID int) (float64, float64, error) {

	var current, withdrawn float64

	// Запрашиваем список ордеров пользователя из базы данных
	err := d.Pool.QueryRow(context.Background(), `
		WITH sum_orders
     AS (SELECT COALESCE(Sum(accrual), 0) AS summ
         FROM   sp_orders spo
                JOIN sp_users spu using(uid)
         WHERE
     spu.uid =
     $1),
     sum_withdrawn
     AS (SELECT COALESCE(Sum(withdrawn_value), 0) AS summ
         FROM   sp_withdrawn_history spo
                JOIN sp_users spu using(uid)
         WHERE
     spu.uid =
     $2)
SELECT sum_orders.summ - sum_withdrawn.summ AS CURRENT,
       sum_withdrawn.summ                   AS withdrawn
FROM   sum_orders,
       sum_withdrawn
	`, userID, userID).Scan(&current, &withdrawn)

	if err != nil {
		return 0, 0, fmt.Errorf("failed to query user balance: %w", err)
	}

	return current, withdrawn, nil
}

func (d *PgDB) UserBalanceWithdraw(userID int, withdrawValue float64, orderID string) (bool, error) {

	var current float64
	var withdrawnID int

	// Запрашиваем список ордеров пользователя из базы данных
	err := d.Pool.QueryRow(context.Background(), `
		WITH sum_orders
     AS (SELECT COALESCE(Sum(accrual), 0) AS summ
         FROM   sp_orders spo
                JOIN sp_users spu using(uid)
         WHERE
     spu.uid =
     $1),
     sum_withdrawn
     AS (SELECT COALESCE(Sum(withdrawn_value), 0) AS summ
         FROM   sp_withdrawn_history spo
                JOIN sp_users spu using(uid)
         WHERE
     spu.uid =
     $2)
SELECT sum_orders.summ - sum_withdrawn.summ AS CURRENT
       
FROM   sum_orders,
       sum_withdrawn
	`, userID, userID).Scan(&current)

	if err != nil {
		return false, fmt.Errorf("failed to query user balance: %w", err)
	}

	if current < withdrawValue {
		return false, nil
	}

	err = d.Pool.QueryRow(context.Background(), `
		INSERT INTO sp_withdrawn_history (uid, withdrawn_value, order_id) values ($1, $2, $3) returning withdrawn_id
`, userID, withdrawValue, orderID).Scan(&withdrawnID)
	if err != nil {
		return false, fmt.Errorf("failed to withdraw: %w", err)
	}

	return true, nil
}

func (d *PgDB) UserWithdrawals(userID int) ([]Withdrawal, error) {
	rows, err := d.Pool.Query(context.Background(), `
		SELECT swh.order_id, swh.withdrawn_value, swh.created_time FROM sp_withdrawn_history swh JOIN sp_users spu USING(uid) WHERE spu.uid=$1
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to query user withdrawals: %w", err)
	}
	defer rows.Close()
	var orderInt int
	var withdrawals []Withdrawal
	for rows.Next() {
		var withdrawal Withdrawal
		err := rows.Scan(&orderInt, &withdrawal.Sum, &withdrawal.Date)
		if err != nil {
			return nil, fmt.Errorf("failed to scan withdrawal row: %w", err)
		}
		withdrawal.Order = strconv.Itoa(orderInt)
		withdrawals = append(withdrawals, withdrawal)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over withdrawal rows: %w", err)
	}

	return withdrawals, nil
}

func (d *PgDB) UserGetOrder(userID int, orderID int, limit int) (Order, error) {
	var count, requesID int

	var order Order
	err := d.Pool.QueryRow(context.Background(), `
		SELECT count(1) FROM sp_requests_history srh JOIN sp_users spu USING(uid) WHERE spu.uid=$1 and srh.created_time>NOW()-'1m'::interval
	`, userID).Scan(&count)
	if err != nil {
		return order, fmt.Errorf("failed to query order information: %w", err)
	}

	if count >= limit {
		return order, fmt.Errorf("too many requests")
	}
	err = d.Pool.QueryRow(context.Background(), `
		INSERT INTO sp_requests_history (uid) values ($1) returning request_id
`, userID).Scan(&requesID)

	if err != nil {
		return order, fmt.Errorf("failed to save request_id")
	}

	if requesID == 0 {
		return order, fmt.Errorf("wtf")
	}

	err = d.Pool.QueryRow(context.Background(), `
	SELECT so.order_value, sst.status_value, so.accrual from sp_orders so join sp_users spu using(uid) join sp_statuses sst using(status_id) where spu.uid=$1 and order_value=$2
`, userID, orderID).Scan(&order.Order, &order.Status, &order.Accrual)
	if err != nil {

		return order, fmt.Errorf("failed to get order info")
	}

	return order, nil
}

func (d *PgDB) Ping() (int, error) {
	var i int
	err := d.Pool.QueryRow(context.Background(), `SELECT 1`).Scan(&i)
	if err != nil {
		fmt.Println(err)
		return 0, fmt.Errorf("failed to select")
	}

	return i, nil
}
