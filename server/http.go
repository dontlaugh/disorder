package server

import (
	"sync"

	badger "github.com/dgraph-io/badger/v4"
)

// DB is a package global pointer. Call Init before you use it. Better yet,
// use it via safe accessor functions.
var DB *badger.DB

// Init initializes package globals. It can only be called once. Pass it a function
// that takes an error and logs it with your logger.
func Init(errLogFn func(error)) {
	var once sync.Once
	once.Do(func() {
		opt := badger.DefaultOptions("").WithInMemory(true)
		db, err := badger.Open(opt)
		if err != nil {
			errLogFn(err)
		}
		DB = db
	})
}
