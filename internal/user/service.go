package user

import (
	"database/sql"

	"mini-escrow/internal/ledger"
)

type Service struct {
	db         *sql.DB
	repo       *Repository
	ledgerRepo *ledger.Repository
}

func NewService(db *sql.DB) *Service {
	return &Service{
		db:         db,
		repo:       NewRepository(db),
		ledgerRepo: ledger.NewRepository(db),
	}
}

func (s *Service) CreateUser(name string) (*User, error) {
	return s.repo.Create(name)
}

// Deposit: 사용자의 잔액을 충전하는 간단한 유스케이스.
// ledger에 DEPOSIT 레코드를 추가해서 잔액을 증가시킨다.
func (s *Service) Deposit(userID int64, amount int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// 단순 충전이므로 orderID는 nil
	err = s.ledgerRepo.Insert(tx, userID, amount, ledger.Deposit, nil)
	if err != nil {
		return err
	}

	err = tx.Commit()
	return err
}

// GetBalance: ledger 기반으로 사용자의 현재 잔액을 조회한다.
func (s *Service) GetBalance(userID int64) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}

	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	balance, err := s.ledgerRepo.GetBalance(tx, userID)
	if err != nil {
		return 0, err
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}

	return balance, nil
}

