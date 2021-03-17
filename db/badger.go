package db

import "github.com/dgraph-io/badger/v3"

func StartDBService() (*badger.DB, error) {
	db, err := badger.Open(badger.DefaultOptions("/rp/badger"))
	if err != nil {
		return nil, err
	}
	return db, nil
}

func StopDBService(db *badger.DB) {
	db.Close()
}
