package ledger

import "database/sql"

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

	query := `
		INSERT INTO ledger (user_id, amount, type, order_id)
		VALUES (?, ?, ?, ?)
	`

	_, err := tx.Exec(query, userID, amount, entryType, orderID)
	return err
}