## Mini Escrow 구조 정리 (공부용 메모)

이 문서는 `README.md`보다 더 자세하게, **프로젝트 구조와 파일들의 관계, 전체 흐름**을 이해하기 위한 공부용 노트입니다.

---

## 1. 전체 큰 그림

- **도메인**
  - `internal/ledger`: 돈의 흐름(거래 내역) 기록
  - `internal/order`: 에스크로 주문과 상태 머신
  - `internal/user`: 사용자와 잔액 조회/충전
- **인프라**
  - `internal/db/mysql.go`: MySQL 연결 (DSN, Ping)
- **HTTP 레이어**
  - `cmd/server/main.go`: Gin HTTP 서버, 각 서비스 연결 및 라우트 정의
- **프론트엔드**
  - `frontend/*`: React + Vite 기반 UI (유저/입금/주문/Fund/Confirm/Cancel)
- **테스트**
  - `internal/order/service_test.go`: sqlmock 기반 유닛 테스트
  - `internal/order/integration_test.go`: 실제 MySQL 기반 동시성/idem 통합 테스트

데이터는 모두 **MySQL**에 저장되고, `ledger` 테이블을 기준으로 잔액을 계산합니다.

---

## 2. DB 스키마와 의미 (`schema.sql`)

```1:25:c:\Users\NHSキムスミン\Desktop\mini-escrow\schema.sql
CREATE TABLE IF NOT EXISTS users (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  name VARCHAR(255) NOT NULL
);

CREATE TABLE IF NOT EXISTS orders (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  buyer_id BIGINT NOT NULL,
  seller_id BIGINT NOT NULL,
  amount BIGINT NOT NULL,
  status VARCHAR(32) NOT NULL,
  version BIGINT NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS ledger (
  id BIGINT AUTO_INCREMENT PRIMARY KEY,
  user_id BIGINT NOT NULL,
  amount BIGINT NOT NULL,
  type VARCHAR(32) NOT NULL,
  order_id BIGINT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_ledger_user_created_at
  ON ledger (user_id, created_at);

CREATE TABLE IF NOT EXISTS idempotency_keys (
  id VARCHAR(100) PRIMARY KEY,
  order_id BIGINT NOT NULL,
  operation VARCHAR(32) NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

- **users**
  - 단순 유저 정보 (id, name)
- **orders**
  - 에스크로 주문
  - `status`: CREATED / FUNDED / CONFIRMED / CANCELLED
  - `version`: optimistic lock용 버전 (갱신될 때마다 +1)
- **ledger**
  - 모든 돈 이동 기록 (불변)
  - `amount`: +면 입금, -면 출금
  - `type`: DEPOSIT / WITHDRAW
  - `order_id`: 어떤 주문 때문에 발생한 거래인지 (nullable)
  - `user_id` + `created_at` 인덱스: 사용자별 거래 조회 최적화
- **idempotency_keys**
  - `FundOrder` 요청의 idem 키 저장
  - 같은 `id`로 INSERT가 두 번 들어오면 **한 번만 성공** → idempotent 보장

---

## 3. Ledger 레이어 (`internal/ledger`)

### 3.1 도메인 (`domain.go`)

```1:16:c:\Users\NHSキムスミン\Desktop\mini-escrow\internal\ledger\domain.go
type EntryType string

const (
	Deposit  EntryType = "DEPOSIT"
	Withdraw EntryType = "WITHDRAW"
)

type Entry struct {
	ID      int64
	UserID  int64
	Amount  int64
	Type    EntryType
	OrderID *int64
}
```

- ledger의 한 줄을 나타내는 도메인 모델.
- 실무처럼 `DEPOSIT` / `WITHDRAW` 타입을 명시적으로 관리.

### 3.2 Repository (`repository.go`)

```5:40:c:\Users\NHSキムスミン\Desktop\mini-escrow\internal\ledger\repository.go
type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

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

func (r *Repository) Insert(
	tx *sql.Tx,
	userID int64,
	amount int64,
	entryType EntryType,
	orderID *int64,
) error {
	...
}
```

- **항상 `*sql.Tx`를 사용**해서 트랜잭션 안에서만 동작하도록 설계.
- 잔액은 `SUM(amount)`로 계산 → ledger 기반 설계.

---

## 4. Order 레이어 (`internal/order`)

### 4.1 도메인 (`domain.go`): 상태 머신

```5:44:c:\Users\NHSキムスミン\Desktop\mini-escrow\internal\order\domain.go
type Status string

const (
	Created   Status = "CREATED"
	Funded    Status = "FUNDED"
	Confirmed Status = "CONFIRMED"
	Cancelled Status = "CANCELLED"
)

type Order struct {
	ID       int64
	BuyerID  int64
	SellerID int64
	Amount   int64
	Status   Status
	Version  int64
}

func (o *Order) Fund() error    { ... }   // CREATED → FUNDED
func (o *Order) Confirm() error { ... }   // FUNDED → CONFIRMED
func (o *Order) Cancel() error  { ... }   // FUNDED → CANCELLED
```

- **상태 변경 규칙은 도메인 메서드의 책임** (Service에서는 `Fund()`, `Confirm()`, `Cancel()` 호출만).

### 4.2 Repository (`repository.go`): 락 + optimistic lock

```8:59:c:\Users\NHSキムスミン\Desktop\mini-escrow\internal\order\repository.go
func (r *Repository) GetByID(tx *sql.Tx, id int64) (*Order, error) {
	query := `
		SELECT id, buyer_id, seller_id, amount, status, version
		FROM orders
		WHERE id = ?
		FOR UPDATE
	`
	...
}

func (r *Repository) UpdateStatus(tx *sql.Tx, o *Order) error {
	query := `
		UPDATE orders
		SET status = ?, version = version + 1
		WHERE id = ? AND version = ?
	`
	...
}
```

- **비관적 락**: `SELECT ... FOR UPDATE`로 row lock.
- **낙관적 락**: `WHERE id=? AND version=?` + `RowsAffected==0` → concurrent update 감지.

### 4.3 Service (`service.go`): 트랜잭션 경계

- `CreateOrder`: 단순 주문 생성.
- `FundOrderWithKey`: **idempotency 키를 사용한 Fund 구현**.
- `FundOrder`: 기존 호출자용 wrapper (`key=""`).
- `ConfirmOrder`, `CancelOrder`: 정산/환불 트랜잭션.

핵심 패턴(모든 메서드 공통):

```28:93:c:\Users\NHSキムスミン\Desktop\mini-escrow\internal\order\service.go
tx, err := s.db.Begin()
if err != nil { ... }
defer func() {
	if err != nil {
		_ = tx.Rollback()
	}
}()
// ... 도메인/ledger 작업 ...
err = tx.Commit()
return err
```

- **하나의 트랜잭션 안에서**:
  - 주문 row lock
  - balance 체크 (ledger SUM)
  - ledger 삽입 (WITHDRAW/DEPOSIT)
  - 주문 상태 업데이트
  - 커밋 / 에러 시 롤백

#### 4.3.1 Fund + Idempotency

- `FundOrderWithKey(orderID, platformID, key)`:
  1. 트랜잭션 시작
  2. `idempotency_keys`에 `(key, order_id, "FUND")` INSERT 시도
     - 중복 키 에러 → 해당 주문이 이미 `FUNDED`면 **성공으로 간주** (idempotent 처리)
  3. 정상 INSERT면 기존 Fund 트랜잭션 로직 수행

```36:88:c:\Users\NHSキムスミン\Desktop\mini-escrow\internal\order\service.go
if key != "" {
	_, err = tx.Exec(
		`INSERT INTO idempotency_keys (id, order_id, operation) VALUES (?, ?, 'FUND')`,
		key,
		orderID,
	)
	if err != nil {
		var status string
		row := tx.QueryRow(`SELECT status FROM orders WHERE id = ?`, orderID)
		if scanErr := row.Scan(&status); scanErr != nil {
			return scanErr
		}
		if status == string(Funded) {
			err = tx.Commit()
			return err
		}
		return err
	}
}
```

---

## 5. User 레이어 (`internal/user`)

- 단순히 **유저 관리 + 충전 + 잔액 조회**를 제공해서 UI/실험을 편하게 하는 레이어.

구성:

- `domain.go`: `User{ID, Name}`
- `repository.go`:
  - `Create(name)` → `users` INSERT
  - `GetByID(id)`
- `service.go`:
  - `CreateUser(name)`
  - `Deposit(userID, amount)` → ledger에 DEPOSIT 삽입 (트랜잭션)
  - `GetBalance(userID)` → ledger SUM (트랜잭션)

---

## 6. HTTP 레이어 (`cmd/server/main.go`)

Gin 서버에서 각 서비스들을 엮습니다.

핵심 라우트:

- 헬스체크
  - `GET /health`
- User 관련
  - `POST /users` → 유저 생성
  - `POST /users/:id/deposit` → 충전
  - `GET /users/:id/balance` → 잔액 조회
- Order 관련
  - `POST /orders` → 주문 생성
  - `POST /orders/:id/fund` → Fund 트랜잭션
  - `POST /orders/:id/confirm` → Confirm 트랜잭션
  - `POST /orders/:id/cancel` → Cancel 트랜잭션

각 handler는 **Service 메서드만 호출**하고, HTTP 코드/에러 메시지 매핑만 책임집니다.

---

## 7. 테스트 구조

### 7.1 유닛 테스트 (`internal/order/service_test.go`)

- `sqlmock` 사용:
  - Confirm/CANCEL/Fund가 올바른 순서로:
    - `BEGIN`
    - `SELECT ... FOR UPDATE`
    - `SELECT SUM(amount)`
    - `INSERT INTO ledger` (WITHDRAW/DEPOSIT)
    - `UPDATE orders ... WHERE id=? AND version=?`
    - `COMMIT` 또는 `ROLLBACK`
  - 잔액 부족 / 잘못된 상태 / 플랫폼 잔액 부족 등 에러 케이스 검증.

### 7.2 통합 테스트 (`internal/order/integration_test.go`)

```11:106:c:\Users\NHSキムスミン\Desktop\mini-escrow\internal\order\integration_test.go
// ESCROW_INTEGRATION_TEST=1 환경 변수 설정 시에만 실제 MySQL에 붙어서 실행됨.
```

두 가지 주요 테스트:

1. **동시성 테스트** (`TestFundOrder_Concurrency_WithRealDB`)
   - 같은 주문에 대해 goroutine 2개가 동시에 `FundOrder` 호출.
   - 기대:
     - 성공 1번, 실패 1번 (optimistic/pessimistic lock 조합으로 하나만 성공)
     - 최종 주문 상태: `FUNDED`, `version=1`.
2. **Idempotency 테스트** (`TestFundOrder_IdempotentKey_WithRealDB`)
   - 같은 `orderID`, 같은 `key`로 `FundOrderWithKey`를 2번 동시에 호출.
   - 기대:
     - 두 호출 모두 "성공"으로 간주 (`successCnt == 2`)
     - 실제로는 한 번만 새로운 idempotency 키가 INSERT되고, 주문은 `FUNDED` 상태.

---

## 8. 프론트엔드 구조 (`frontend`)

주요 파일:

- `package.json` / `vite.config.ts` / `tsconfig.json` / `index.html`
- `src/main.tsx` – React 엔트리
- `src/App.tsx` – 메인 화면
- `src/styles.css` – 블랙/화이트 다크 스타일

`App.tsx`에서 제공하는 흐름:

1. User 생성 → `POST /users`
2. Deposit → `POST /users/:id/deposit` + `GET /users/:id/balance`
3. Order 생성 → `POST /orders`
4. Fund / Confirm / Cancel 버튼 → 각각 `/orders/:id/fund|confirm|cancel`

이 UI를 통해 **실제 트랜잭션 흐름과 상태 변화를 손으로 체험**할 수 있습니다.

---

## 9. 정리: 이 구조가 보여주는 것

이 프로젝트는 다음 역량을 증명합니다.

- ledger 기반 잔액 계산과 회계적 사고
- Service 레이어 중심의 트랜잭션 경계 설계
- `SELECT ... FOR UPDATE`를 통한 비관적 락
- version 컬럼을 이용한 낙관적 락 및 동시성 제어
- idempotency 키를 활용한 API 레벨 “한 번만 처리” 보장
- sqlmock + 실제 MySQL을 모두 사용하는 테스트 전략
- React 기반의 간단하지만 실무형 구조의 UI로 end-to-end 플로우 확인

이 파일은 필요한 부분을 복습하거나 면접 준비용으로 읽기 좋게 만드는 것을 목표로 했습니다.

