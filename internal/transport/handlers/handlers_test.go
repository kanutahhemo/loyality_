package handlers_test

import (
	"context"
	"github.com/driftprogramming/pgxpoolmock"
	"github.com/jackc/pgx/v4"
)

type MockPoolWrapper struct {
	mockPool *pgxpoolmock.MockPgxPool
}

func (m *MockPoolWrapper) Close() {
	m.mockPool.Close()
}

func (m *MockPoolWrapper) Query(ctx context.Context, sql string, arguments ...interface{}) (pgx.Rows, error) {
	return m.mockPool.Query(ctx, sql, arguments...)
}

func (m *MockPoolWrapper) QueryRow(ctx context.Context, sql string, arguments ...interface{}) pgx.Row {
	return m.mockPool.QueryRow(ctx, sql, arguments...)
}

/*
func TestUserRegisterHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Создаем мок-объект pgxpool.Pool
	mockPool := pgxpoolmock.NewMockPgxPool(ctrl)
	poolWrapper := &MockPoolWrapper{mockPool: mockPool}

	// Устанавливаем ожидание вызова Exec с определенными аргументами
	mockPool.EXPECT().Exec(gomock.Any(), "insert into sp_users (login, password) values ($1, $2) returning uid", "testlogin", "testpassword").Return(nil, nil)

	// Создаем экземпляр PgDB с реальным pgxpool.Pool и заменяем его на мок-объект
	pgdb := &database.PgDB{
		Pool: poolWrapper,
	}

	// Создаем тестовый HTTP-запрос
	reqBody := `{"login": "testlogin", "password": "testpassword"}`
	req := httptest.NewRequest("POST", "/register", strings.NewReader(reqBody))
	rr := httptest.NewRecorder()

	// Вызываем хендлер
	handler := handlers.UserRegister(*pgdb)
	handler.ServeHTTP(rr, req)

	// Проверяем статус-код и ожидаемый ответ
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, `{"msg":"Registration Successful","err":""}`, rr.Body.String())
}
*/
