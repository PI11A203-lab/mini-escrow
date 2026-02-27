## Mini Escrow 프로젝트 개요

이 프로젝트는 **에스크로(escrow) 결제 흐름**을 학습·증명하기 위한 최소한의 서버입니다.

핵심 시나리오:

- **Fund**: 구매자가 플랫폼에 돈을 예치 (CREATED → FUNDED)
- **Confirm**: 판매자 정산 (FUNDED → CONFIRMED)
- **Cancel**: 구매자 환불 (FUNDED → CANCELLED)

모든 돈 이동은 **`ledger` 테이블 기반**으로 이루어지며, 단순 CRUD가 아니라 **트랜잭션·동시성·락 전략**을 중심으로 설계되어 있습니다.

---

## Ledger 기반 설계

- **단일 balance 컬럼을 두지 않고**, `ledger` 테이블의 `amount` 합으로 잔액을 계산합니다.
- 한 번 적힌 `ledger` 레코드는 변경하지 않는 것을 전제로 하여, **회계 추적 가능성(감사 가능성)** 을 보장합니다.
- 잔액 조회는 다음과 같이 수행합니다.

```12:22:c:\Users\NHSキム스ミン\Desktop\mini-escrow\internal\ledger\repository.go
func (r *Repository) GetBalance(tx *sql.Tx, userID int64) (int64, error) {
	query := `
		SELECT IFNULL(SUM(amount),0)
		FROM ledger
		WHERE user_id = ?
	`

	var balance int64
	err := tx.QueryRow(query, userID).Scan(&balance)
	return balance, err
}
```

이 방식은 **정합성을 최우선**으로 두는 전통적인 금융/회계 시스템 패턴과 같습니다.

---

## 동시성 전략

### 1. 비관적 락 (Pessimistic Lock)

주문을 읽을 때 다음과 같이 `SELECT ... FOR UPDATE` 를 사용합니다.

```13:21:c:\Users\NHSキムスミン\Desktop\mini-escrow\internal\order\repository.go
func (r *Repository) GetByID(tx *sql.Tx, id int64) (*Order, error) {
	query := `
		SELECT id, buyer_id, seller_id, amount, status, version
		FROM orders
		WHERE id = ?
		FOR UPDATE
	`

	row := tx.QueryRow(query, id)
	...
}
```

- 같은 주문에 대한 **동시 Fund/Confirm/Cancel 요청**이 들어오면,
  - 첫 번째 트랜잭션이 row lock을 잡고 처리
  - 두 번째 트랜잭션은 첫 번째가 끝날 때까지 블로킹

### 2. 낙관적 락 (Optimistic Lock)

`orders` 테이블에 `version` 컬럼을 두고, 상태 변경 시 다음과 같이 갱신합니다.

```32:41:c:\Users\NHSキムスミン\Desktop\mini-escrow\internal\order\repository.go
func (r *Repository) UpdateStatus(tx *sql.Tx, o *Order) error {
	query := `
		UPDATE orders
		SET status = ?, version = version + 1
		WHERE id = ? AND version = ?
	`

	res, err := tx.Exec(query, o.Status, o.ID, o.Version)
	if err != nil {
		return err
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return errors.New("concurrent update detected")
	}

	o.Version++
	return nil
}
```

- `WHERE id = ? AND version = ?` 조건을 통해, **읽었을 때와 동일한 버전일 때만** 업데이트가 성공합니다.
- `RowsAffected == 0` 이면 누군가 먼저 변경한 것이므로, **동시 업데이트를 감지**하고 에러를 반환합니다.
- 이 패턴으로 API 레벨에서의 **idempotency 보장**의 기반을 마련할 수 있습니다.

---

## 트랜잭션 경계 (Service 레이어)

모든 돈 이동은 **Service 레이어의 하나의 트랜잭션** 안에서 처리됩니다.

예: Fund (구매자 예치)

```24:89:c:\Users\NHSキムスミン\Desktop\mini-escrow\internal\order\service.go
func (s *Service) FundOrder(orderID int64, platformID int64) (err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// 1️⃣ 주문 조회 + row lock
	order, err := s.orderRepo.GetByID(tx, orderID)
	...

	// 2️⃣ 잔액 확인
	balance, err := s.ledgerRepo.GetBalance(tx, order.BuyerID)
	...

	// 3️⃣ 구매자 돈 차감
	// 4️⃣ 플랫폼 돈 증가
	// 5️⃣ 주문 상태 변경 (CREATED → FUNDED)
	// 6️⃣ commit
	err = tx.Commit()
	return err
}
```

같은 방식으로 Confirm/Cancel 에서도:

- 플랫폼 ↔ 판매자/구매자 ledger 이동
- 주문 상태 전이
- 모두 하나의 트랜잭션 안에서 처리되어, **중간에 에러 나면 전체 롤백**됩니다.

---

## 테스트 전략

### 1. 단위 테스트 (sqlmock)

`sqlmock`을 사용해 다음을 검증합니다.

- 잔액 부족 시 롤백되는가?
- 잘못된 상태에서 Fund/Confirm/Cancel 호출 시 에러/롤백되는가?
- ledger에 올바른 금액·유형으로 INSERT 되는가?
- `UPDATE orders` 가 기대하는 쿼리와 파라미터로 호출되는가?

### 2. 통합 테스트 (실제 MySQL, 동시성)

`ESCROW_INTEGRATION_TEST=1` 환경 변수 설정 시, 실제 MySQL에 붙는 통합 테스트를 실행할 수 있습니다.

```1:84:c:\Users\NHSキム스ミン\Desktop\mini-escrow\internal\order\integration_test.go
func TestFundOrder_Concurrency_WithRealDB(t *testing.T) {
	if os.Getenv("ESCROW_INTEGRATION_TEST") != "1" {
		t.Skip("ESCROW_INTEGRATION_TEST != 1, skipping integration test")
	}

	database, err := db.NewDB()
	...

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		errCh <- svc.FundOrder(orderID, platformID)
	}()

	go func() {
		defer wg.Done()
		errCh <- svc.FundOrder(orderID, platformID)
	}()

	wg.Wait()
	...

	if successCnt != 1 {
		t.Fatalf("expected exactly 1 success, got %d success(es), %d fail(s)", successCnt, failCnt)
	}
}
```

- 같은 주문에 대해 **동시에 FundOrder를 두 번 호출**했을 때,
  - 정확히 한 번만 성공하는지,
  - 최종 주문 상태가 `FUNDED` 이고 버전이 1인지,
  - 나머지 호출은 도메인/락/버전 검증으로 실패하는지 확인합니다.

---

## 앞으로 확장 아이디어

- **Deadlock 실험**: 일부러 교차 락을 만들어 MySQL의 데드락 동작을 관찰하고, 재시도 전략 설계.
- **Running Balance 컬럼 추가**: ledger-only 계산과 `running_balance` 병행 유지 전략 비교.
- **API 레벨 idempotency key**: 클라이언트가 `Idempotency-Key` 를 보내면, 같은 키로 동일한 요청을 한 번만 처리하도록 확장.

이 문서를 기반으로, 면접/블로그 등에서 다음 질문에 답할 수 있도록 설계되었습니다.

- 왜 ledger 기반으로 잔액을 계산했는가?
- 동시성 문제는 어떻게 막았는가?
- 트랜잭션 경계는 어디까지인가?
- rollback이 어떻게 보장되는가?

