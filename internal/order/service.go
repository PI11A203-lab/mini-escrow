package order

import (
	"database/sql"
	"errors"

	"mini-escrow/internal/ledger"
)

type Service struct {
	db         *sql.DB
	orderRepo  *Repository
	ledgerRepo *ledger.Repository
}

func NewService(db *sql.DB) *Service {
	return &Service{
		db:         db,
		orderRepo:  NewRepository(db),
		ledgerRepo: ledger.NewRepository(db),
	}
}

func (s *Service) CreateOrder(buyerID, sellerID, amount int64) (*Order, error) {
	return s.orderRepo.Create(buyerID, sellerID, amount)
}

// FundOrderWithKey:
// idempotency 키를 사용하는 Fund 구현.
// 같은 key로 여러 번 호출되면 한 번만 실제로 돈이 이동하고,
// 이후 호출은 이미 처리된 것으로 간주한다.
func (s *Service) FundOrderWithKey(orderID int64, platformID int64, key string) (err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// 0️⃣ idempotency 키 등록 (있을 경우)
	if key != "" {
		_, err = tx.Exec(
			`INSERT INTO idempotency_keys (id, order_id, operation) VALUES (?, ?, 'FUND')`,
			key,
			orderID,
		)
		if err != nil {
			// 이미 처리된 키라면, 해당 주문이 FUNDED 상태인지 확인하고 그대로 성공으로 간주
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

	// 1️⃣ 주문 조회 + row lock
	order, err := s.orderRepo.GetByID(tx, orderID)
	if err != nil {
		return err
	}

	// 2️⃣ 잔액 확인
	balance, err := s.ledgerRepo.GetBalance(tx, order.BuyerID)
	if err != nil {
		return err
	}

	if balance < order.Amount {
		return errors.New("insufficient balance")
	}

	// 3️⃣ 구매자 돈 차감
	err = s.ledgerRepo.Insert(
		tx,
		order.BuyerID,
		-order.Amount,
		ledger.Withdraw,
		&order.ID,
	)
	if err != nil {
		return err
	}

	// 4️⃣ 플랫폼 돈 증가
	err = s.ledgerRepo.Insert(
		tx,
		platformID,
		order.Amount,
		ledger.Deposit,
		&order.ID,
	)
	if err != nil {
		return err
	}

	// 5️⃣ 상태 변경
	err = order.Fund()
	if err != nil {
		return err
	}

	err = s.orderRepo.UpdateStatus(tx, order)
	if err != nil {
		return err
	}

	// 6️⃣ commit
	err = tx.Commit()
	return err
}

// 기존 호출자들을 위한 wrapper (idempotency 키 없이 사용)
func (s *Service) FundOrder(orderID int64, platformID int64) (err error) {
	return s.FundOrderWithKey(orderID, platformID, "")
}

// ConfirmOrder:
// FUNDED 상태의 주문에 대해
// 플랫폼 → 판매자 정산, 주문 상태를 CONFIRMED로 변경한다.
func (s *Service) ConfirmOrder(orderID int64, platformID int64) (err error) {
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
	if err != nil {
		return err
	}

	// 2️⃣ 상태 전이 (FUNDED → CONFIRMED)
	if err = order.Confirm(); err != nil {
		return err
	}

	// 3️⃣ 플랫폼 잔액 확인
	platformBalance, err := s.ledgerRepo.GetBalance(tx, platformID)
	if err != nil {
		return err
	}
	if platformBalance < order.Amount {
		return errors.New("insufficient platform balance")
	}

	// 4️⃣ 플랫폼 → 판매자 정산
	// 플랫폼 출금
	err = s.ledgerRepo.Insert(
		tx,
		platformID,
		-order.Amount,
		ledger.Withdraw,
		&order.ID,
	)
	if err != nil {
		return err
	}

	// 판매자 입금
	err = s.ledgerRepo.Insert(
		tx,
		order.SellerID,
		order.Amount,
		ledger.Deposit,
		&order.ID,
	)
	if err != nil {
		return err
	}

	// 5️⃣ 상태 저장
	if err = s.orderRepo.UpdateStatus(tx, order); err != nil {
		return err
	}

	// 6️⃣ commit
	err = tx.Commit()
	return err
}

// CancelOrder:
// FUNDED 상태의 주문에 대해
// 플랫폼 → 구매자 환불, 주문 상태를 CANCELLED로 변경한다.
func (s *Service) CancelOrder(orderID int64, platformID int64) (err error) {
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
	if err != nil {
		return err
	}

	// 2️⃣ 상태 전이 (FUNDED → CANCELLED)
	if err = order.Cancel(); err != nil {
		return err
	}

	// 3️⃣ 플랫폼 잔액 확인
	platformBalance, err := s.ledgerRepo.GetBalance(tx, platformID)
	if err != nil {
		return err
	}
	if platformBalance < order.Amount {
		return errors.New("insufficient platform balance")
	}

	// 4️⃣ 플랫폼 → 구매자 환불
	// 플랫폼 출금
	err = s.ledgerRepo.Insert(
		tx,
		platformID,
		-order.Amount,
		ledger.Withdraw,
		&order.ID,
	)
	if err != nil {
		return err
	}

	// 구매자 입금
	err = s.ledgerRepo.Insert(
		tx,
		order.BuyerID,
		order.Amount,
		ledger.Deposit,
		&order.ID,
	)
	if err != nil {
		return err
	}

	// 5️⃣ 상태 저장
	if err = s.orderRepo.UpdateStatus(tx, order); err != nil {
		return err
	}

	// 6️⃣ commit
	err = tx.Commit()
	return err
}

