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

// Instrument — найденный инструмент (тип = категория таблицы).
type Instrument struct {
	Ticker string
	Name   string
	Type   string
}

func (s *Storage) SaveInstrument(instr Instrument) error {
	_, err := s.db.Exec(
		"INSERT INTO instruments (ticker, name, type) VALUES (?, ?, ?) ON CONFLICT(ticker) DO NOTHING",
		instr.Ticker, instr.Name, instr.Type,
	)
	return err
}

func (s *Storage) LoadInstruments() ([]Instrument, error) {
	rows, err := s.db.Query("SELECT ticker, name, type FROM instruments")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Instrument
	for rows.Next() {
		var i Instrument
		if err := rows.Scan(&i.Ticker, &i.Name, &i.Type); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (s *Storage) Close() error {
	return s.db.Close()
}
