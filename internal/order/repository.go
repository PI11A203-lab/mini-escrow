package order

import (
	"database/sql"
	"errors"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetByID(tx *sql.Tx, id int64) (*Order, error) {
	query := `
		SELECT id, buyer_id, seller_id, amount, status, version
		FROM orders
		WHERE id = ?
		FOR UPDATE
	`

	row := tx.QueryRow(query, id)

	o := &Order{}
	err := row.Scan(&o.ID, &o.BuyerID, &o.SellerID, &o.Amount, &o.Status, &o.Version)
	if err != nil {
		return nil, err
	}

	return o, nil
}

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

	// 메모리 상 버전도 증가
	o.Version++

	return nil
}

