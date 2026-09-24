package storage

import (
	"database/sql"
	"time"

	_ "modernc.org/sqlite"
)

type Storage struct {
	db *sql.DB
}

func New(path string) (*Storage, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	return &Storage{db: db}, nil
}

func (s *Storage) Init() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS instruments (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ticker TEXT UNIQUE,
		name TEXT,
		type TEXT
	)`)
	if err != nil {
		return err
	}

	_, err = s.db.Exec(`CREATE TABLE IF NOT EXISTS volatility_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		ticker TEXT,
		date TEXT,
		volatility REAL,
		price REAL
	)`)
	return err
}

func (s *Storage) SaveVolatility(ticker string, vol, price float64) error {
	_, err := s.db.Exec(
		"INSERT INTO volatility_history (ticker, date, volatility, price) VALUES (?, ?, ?, ?)",
		ticker, time.Now().Format(time.RFC3339), vol, price,
	)
	return err
}

func (s *Storage) Close() error {
	return s.db.Close()
}
