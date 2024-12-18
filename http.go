package disorder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/dgraph-io/badger/v4"
	"github.com/flosch/pongo2/v6"
	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc/metadata"
)

// X is a package global instance. Call Init first.
var X *Xeno

// Init initializes package globals. It can only be called once. Pass it a function
// that takes an error and logs it with your logger.
func Init(logger *logrus.Logger) {

	var once sync.Once
	once.Do(func() {
		if logger == nil {
			logger = logrus.StandardLogger()
			logger.SetFormatter(&logrus.JSONFormatter{})
		}
		opt := badger.DefaultOptions("").WithInMemory(true)
		db, err := badger.Open(opt)
		if err != nil {
			logger.Errorf("badger: %v", err)
		}
		r := mux.NewRouter()
		X = &Xeno{db, r, logger}
		r.Handle("/route/{uuid}", X)
	})
}

// Xeno is our server.
type Xeno struct {
	DB *badger.DB
	R  *mux.Router
	L  *logrus.Logger
}

type ScopedWriter struct {
	txn *badger.Txn
	id  string
	seq *badger.Sequence
	l   *logrus.Logger
}

func (x *Xeno) GetScopedWriterContext(ctx context.Context) (*ScopedWriter, func(), error) {
	var id string
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		idParts := md.Get("x-correlation-id")
		if len(idParts) < 1 {
			return nil, nil, errors.New("missing correlation id")
		}
		id = idParts[0]
	} else {
		id = "unknown"
	}

	return x.GetScopedWriter(id)
}

func (x *Xeno) GetScopedWriter(id string) (*ScopedWriter, func(), error) {
	txn := x.DB.NewTransaction(true)
	seq, err := x.DB.GetSequence([]byte(id), 10_000)
	if err != nil {
		return nil, nil, fmt.Errorf("could not get sequence: %w", err)
	}

	commitFn := func() {
		if err := seq.Release(); err != nil {
			x.L.Errorf("badger sequence: %v", err)
		}
		if err := txn.Commit(); err != nil {
			x.L.Errorf("scoped writer: %v", err)
		}
	}

	var sw ScopedWriter
	sw.txn = txn
	sw.id = id
	sw.seq = seq
	sw.l = x.L

	return &sw, commitFn, nil
}

func (sw *ScopedWriter) Put(msg string) {
	n, err := sw.seq.Next()
	if err != nil {
		sw.l.Errorf("scoped writer next: %v", err)
		return
	}
	entry := Entry{
		Value: msg,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		sw.l.Errorf("failed to marshal entry: %v", err)
		return
	}
	paddedSeq := fmt.Sprintf("%0*d", 5, n)
	key := fmt.Sprintf("prefix/%s/%s", sw.id, paddedSeq)
	if err := sw.txn.Set([]byte(key), data); err != nil {
		sw.l.Errorf("scoped writer set: %v", err)
		return
	}
}

func (sw *ScopedWriter) Putf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	sw.Put(msg)
}

type Entry struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

func (x *Xeno) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	uuid, ok := vars["uuid"]
	if !ok {
		http.Error(w, "missing uuid parameter", http.StatusBadRequest)
		return
	}

	// Prepare prefix for scanning the DB
	prefix := fmt.Sprintf("prefix/%s", uuid)

	var entries []Entry
	err := x.DB.View(func(txn *badger.Txn) error {
		iter := txn.NewIterator(badger.DefaultIteratorOptions)
		defer iter.Close()

		for iter.Seek([]byte(prefix)); iter.ValidForPrefix([]byte(prefix)); iter.Next() {
			item := iter.Item()
			var entry Entry

			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &entry)
			})
			if err != nil {
				entries = append(entries, Entry{Value: fmt.Sprintf("error: %v", err)})
			} else {
				entries = append(entries, entry)
			}
		}
		return nil
	})
	if err != nil {
		x.L.Errorf("failed to read from badger: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Define template
	tpl := pongo2.Must(pongo2.FromString(`
<!DOCTYPE html>
<html>
<head>
    <title>Entries</title>
    <link rel="icon" 
          href="data:image/svg+xml,<svg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'><text y='24' font-size='24'>🦋</text></svg>">
</head>
<body>
    <h1>Entries for {{ uuid }}</h1>
    <ul>
    {% for entry in entries %}
        <li>ID: {{ entry.ID }}, Name: {{ entry.Name }}, Value: {{ entry.Value }}</li>
    {% endfor %}
    </ul>
</body>
</html>
	`))

	ctx := pongo2.Context{
		"uuid":    uuid,
		"entries": entries,
	}

	w.WriteHeader(http.StatusOK)
	if err := tpl.ExecuteWriter(ctx, w); err != nil {
		log.Printf("failed to render template: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	w.Write([]byte("\n"))
}

// RoutePrefixMiddleware returns a middleware that filters requests to only call the Xeno
// server if the path starts with "/route/".
func RoutePrefixMiddleware(xeno *Xeno) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/route/") {
				xeno.R.ServeHTTP(w, r)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
