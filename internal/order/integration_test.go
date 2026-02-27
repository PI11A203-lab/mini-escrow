package order

import (
	"os"
	"sync"
	"testing"

	"mini-escrow/internal/db"
)

// 실제 MySQL을 대상으로 동시성/트랜잭션을 검증하는 통합 테스트.
// 환경 변수 ESCROW_INTEGRATION_TEST 가 "1" 이 아니면 Skip 됩니다.
func TestFundOrder_Concurrency_WithRealDB(t *testing.T) {
	if os.Getenv("ESCROW_INTEGRATION_TEST") != "1" {
		t.Skip("ESCROW_INTEGRATION_TEST != 1, skipping integration test")
	}

	database, err := db.NewDB()
	if err != nil {
		t.Fatalf("failed to connect db: %v", err)
	}
	defer database.Close()

	const (
		orderID    int64 = 1001
		buyerID    int64 = 2001
		sellerID   int64 = 3001
		platformID int64 = 4001
		amount     int64 = 1000
	)

	// 깨끗한 상태로 초기화
	if _, err := database.Exec("DELETE FROM ledger WHERE order_id = ?", orderID); err != nil {
		t.Fatalf("failed to cleanup ledger: %v", err)
	}
	if _, err := database.Exec("DELETE FROM orders WHERE id = ?", orderID); err != nil {
		t.Fatalf("failed to cleanup orders: %v", err)
	}

	// 주문 생성 (CREATED 상태, version=0)
	_, err = database.Exec(
		`INSERT INTO orders (id, buyer_id, seller_id, amount, status, version) VALUES (?, ?, ?, ?, ?, 0)`,
		orderID, buyerID, sellerID, amount, Created,
	)
	if err != nil {
		t.Fatalf("failed to insert order: %v", err)
	}

	// 구매자에게 충분한 잔액 부여
	_, err = database.Exec(
		`INSERT INTO ledger (user_id, amount, type, order_id) VALUES (?, ?, ?, NULL)`,
		buyerID, amount, "DEPOSIT",
	)
	if err != nil {
		t.Fatalf("failed to seed buyer balance: %v", err)
	}

	svc := NewService(database)

	var wg sync.WaitGroup
	wg.Add(2)

	errCh := make(chan error, 2)

	go func() {
		defer wg.Done()
		errCh <- svc.FundOrder(orderID, platformID)
	}()

	go func() {
		defer wg.Done()
		errCh <- svc.FundOrder(orderID, platformID)
	}()

	wg.Wait()
	close(errCh)

	successCnt := 0
	failCnt := 0
	for e := range errCh {
		if e == nil {
			successCnt++
		} else {
			failCnt++
		}
	}

	if successCnt != 1 {
		t.Fatalf("expected exactly 1 success, got %d success(es), %d fail(s)", successCnt, failCnt)
	}

	// 최종 주문 상태 확인
	var status string
	var version int64
	row := database.QueryRow(`SELECT status, version FROM orders WHERE id = ?`, orderID)
	if err := row.Scan(&status, &version); err != nil {
		t.Fatalf("failed to query order: %v", err)
	}

	if status != string(Funded) {
		t.Fatalf("expected status %s, got %s", Funded, status)
	}
	if version != 1 {
		t.Fatalf("expected version 1, got %d", version)
	}
}

