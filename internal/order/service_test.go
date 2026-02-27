package order

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"mini-escrow/internal/ledger"
)

func newTestService(t *testing.T) (*Service, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}

	svc := &Service{
		db:         db,
		orderRepo:  NewRepository(db),
		ledgerRepo: ledger.NewRepository(db),
	}

	cleanup := func() {
		_ = db.Close()
	}

	return svc, mock, cleanup
}

func TestConfirmOrder_Success(t *testing.T) {
	svc, mock, cleanup := newTestService(t)
	defer cleanup()

	orderID := int64(1)
	buyerID := int64(10)
	sellerID := int64(20)
	platformID := int64(100)
	amount := int64(1000)

	// 트랜잭션 시작
	mock.ExpectBegin()

	// 주문 조회 (FOR UPDATE)
	orderRows := sqlmock.NewRows([]string{"id", "buyer_id", "seller_id", "amount", "status", "version"}).
		AddRow(orderID, buyerID, sellerID, amount, Funded, int64(0))
	mock.ExpectQuery("FROM orders").
		WillReturnRows(orderRows)

	// 플랫폼 잔액 조회
	balanceRows := sqlmock.NewRows([]string{"balance"}).
		AddRow(int64(2000))
	mock.ExpectQuery("FROM ledger").
		WillReturnRows(balanceRows)

	// 플랫폼 출금
	mock.ExpectExec("INSERT INTO ledger").
		WithArgs(platformID, -amount, ledger.Withdraw, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// 판매자 입금
	mock.ExpectExec("INSERT INTO ledger").
		WithArgs(sellerID, amount, ledger.Deposit, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(2, 1))

	// 주문 상태 업데이트 (optimistic lock with version)
	mock.ExpectExec("UPDATE orders").
		WithArgs(Confirmed, orderID, int64(0)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	// 커밋
	mock.ExpectCommit()

	if err := svc.ConfirmOrder(orderID, platformID); err != nil {
		t.Fatalf("ConfirmOrder returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}
}

func TestConfirmOrder_InvalidStatus(t *testing.T) {
	svc, mock, cleanup := newTestService(t)
	defer cleanup()

	orderID := int64(1)
	buyerID := int64(10)
	sellerID := int64(20)
	platformID := int64(100)
	amount := int64(1000)

	mock.ExpectBegin()

	// CREATED 상태이면 Confirm()에서 에러
	orderRows := sqlmock.NewRows([]string{"id", "buyer_id", "seller_id", "amount", "status", "version"}).
		AddRow(orderID, buyerID, sellerID, amount, Created, int64(0))
	mock.ExpectQuery("FROM orders").
		WillReturnRows(orderRows)

	// 롤백 기대
	mock.ExpectRollback()

	err := svc.ConfirmOrder(orderID, platformID)
	if err == nil {
		t.Fatalf("expected error for invalid status, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}
}

func TestCancelOrder_Success(t *testing.T) {
	svc, mock, cleanup := newTestService(t)
	defer cleanup()

	orderID := int64(1)
	buyerID := int64(10)
	sellerID := int64(20)
	platformID := int64(100)
	amount := int64(1000)

	_ = sellerID // 현재 Cancel 로직에서는 sellerID를 사용하지 않지만, 스키마 맞추기 위해 포함

	mock.ExpectBegin()

	// FUNDED 주문
	orderRows := sqlmock.NewRows([]string{"id", "buyer_id", "seller_id", "amount", "status", "version"}).
		AddRow(orderID, buyerID, sellerID, amount, Funded, int64(0))
	mock.ExpectQuery("FROM orders").
		WillReturnRows(orderRows)

	// 플랫폼 잔액 조회
	balanceRows := sqlmock.NewRows([]string{"balance"}).
		AddRow(int64(2000))
	mock.ExpectQuery("FROM ledger").
		WillReturnRows(balanceRows)

	// 플랫폼 출금
	mock.ExpectExec("INSERT INTO ledger").
		WithArgs(platformID, -amount, ledger.Withdraw, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	// 구매자 입금
	mock.ExpectExec("INSERT INTO ledger").
		WithArgs(buyerID, amount, ledger.Deposit, sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(2, 1))

	// 주문 상태 업데이트
	mock.ExpectExec("UPDATE orders").
		WithArgs(Cancelled, orderID, int64(0)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	mock.ExpectCommit()

	if err := svc.CancelOrder(orderID, platformID); err != nil {
		t.Fatalf("CancelOrder returned error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}
}

func TestCancelOrder_InsufficientPlatformBalance_Rollback(t *testing.T) {
	svc, mock, cleanup := newTestService(t)
	defer cleanup()

	orderID := int64(1)
	buyerID := int64(10)
	sellerID := int64(20)
	platformID := int64(100)
	amount := int64(1000)

	_ = buyerID
	_ = sellerID

	mock.ExpectBegin()

	// FUNDED 주문
	orderRows := sqlmock.NewRows([]string{"id", "buyer_id", "seller_id", "amount", "status", "version"}).
		AddRow(orderID, buyerID, sellerID, amount, Funded, int64(0))

	mock.ExpectQuery("FROM orders").
		WillReturnRows(orderRows)

	// 플랫폼 잔액 부족
	balanceRows := sqlmock.NewRows([]string{"balance"}).
		AddRow(int64(500))
	mock.ExpectQuery("FROM ledger").
		WillReturnRows(balanceRows)

	// 상태/ledger 변경 없이 롤백만
	mock.ExpectRollback()

	err := svc.CancelOrder(orderID, platformID)
	if err == nil {
		t.Fatalf("expected error due to insufficient platform balance, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}
}

func TestFundOrder_InsufficientBuyerBalance_Rollback(t *testing.T) {
	svc, mock, cleanup := newTestService(t)
	defer cleanup()

	orderID := int64(1)
	buyerID := int64(10)
	sellerID := int64(20)
	platformID := int64(100)
	amount := int64(1000)

	mock.ExpectBegin()

	// CREATED 주문
	orderRows := sqlmock.NewRows([]string{"id", "buyer_id", "seller_id", "amount", "status", "version"}).
		AddRow(orderID, buyerID, sellerID, amount, Created, int64(0))
	mock.ExpectQuery("FROM orders").
		WillReturnRows(orderRows)

	// 구매자 잔액 부족 (500 < 1000)
	balanceRows := sqlmock.NewRows([]string{"balance"}).
		AddRow(int64(500))
	mock.ExpectQuery("FROM ledger").
		WillReturnRows(balanceRows)

	mock.ExpectRollback()

	err := svc.FundOrder(orderID, platformID)
	if err == nil {
		t.Fatalf("expected error due to insufficient buyer balance, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}
}

func TestFundOrder_AlreadyFunded_Error(t *testing.T) {
	svc, mock, cleanup := newTestService(t)
	defer cleanup()

	orderID := int64(1)
	buyerID := int64(10)
	sellerID := int64(20)
	platformID := int64(100)
	amount := int64(1000)

	mock.ExpectBegin()

	// 이미 FUNDED 상태
	orderRows := sqlmock.NewRows([]string{"id", "buyer_id", "seller_id", "amount", "status", "version"}).
		AddRow(orderID, buyerID, sellerID, amount, Funded, int64(0))
	mock.ExpectQuery("FROM orders").
		WillReturnRows(orderRows)

	mock.ExpectRollback()

	err := svc.FundOrder(orderID, platformID)
	if err == nil {
		t.Fatalf("expected error for already funded order, got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}
}

