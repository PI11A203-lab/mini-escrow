package user

import "database/sql"

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(name string) (*User, error) {
	query := `
		INSERT INTO users (name)
		VALUES (?)
	`

	res, err := r.db.Exec(query, name)
	if err != nil {
		return nil, err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &User{
		ID:   id,
		Name: name,
	}, nil
}

func (r *Repository) GetByID(id int64) (*User, error) {
	query := `
		SELECT id, name
		FROM users
		WHERE id = ?
	`

	row := r.db.QueryRow(query, id)

	u := &User{}
	if err := row.Scan(&u.ID, &u.Name); err != nil {
		return nil, err
	}

	return u, nil
}

