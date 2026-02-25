package ledger

import "database/sql"

type Service struct {
	db   *sql.DB
	repo *Repository
}

func NewService(db *sql.DB) *Service {
	return &Service{
		db:   db,
		repo: NewRepository(db),
	}
}