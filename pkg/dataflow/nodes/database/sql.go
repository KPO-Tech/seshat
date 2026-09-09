// Package database provides deterministic node executors for querying
// external databases from a dataflow graph — Postgres, MySQL, SQLite
// (pkg/database/sql, three thin drivers sharing one executor), Redis, and
// MongoDB (native drivers), and Elasticsearch (its own REST API, no client
// SDK needed). Each node resolves its DSN/credential through
// dataflow.Runtime.Secrets rather than taking a raw connection string
// parameter, so a workflow definition never embeds a plaintext password.
package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	_ "github.com/go-sql-driver/mysql" // MySQL driver
	_ "github.com/lib/pq"              // PostgreSQL driver
	// No sqlite driver is blank-imported here, deliberately: the Go sqlite
	// ecosystem has multiple incompatible packages all registering under
	// the same database/sql driver name "sqlite" (mattn/go-sqlite3,
	// modernc.org/sqlite, glebarez/go-sqlite — the last two both build on
	// modernc's engine but each self-register "sqlite" independently),
	// and database/sql.Register panics on a second registration of the
	// same name. Both real consumers of this package (seshat-backend,
	// seshat-server) already depend on glebarez/sqlite for GORM, which
	// registers "sqlite" itself — importing modernc.org/sqlite here too
	// would crash their process at init time (hit and fixed while wiring
	// Phase 4 in seshat-ai: see the seshat-ai commit adding
	// pkg/dataflow/nodes/database.Register to app.go). The NewSQLite node
	// type works as soon as ANY "sqlite" driver is registered somewhere in
	// the embedding process — which, in practice, it almost always already
	// is. sql_test.go blank-imports modernc.org/sqlite for this package's
	// own tests, where no such conflict exists.
	"github.com/KPO-Tech/seshat/pkg/dataflow"
)

// sqlNode is the shared implementation behind the postgres/mysql/sqlite node
// types — all three are database/sql plus a driver import, differing only in
// driverName and how their DSN is resolved. Connections are cached by DSN
// and never proactively closed: *sql.DB is already a pool meant to be kept
// open and reused across calls, and opening a fresh one on every Execute
// (this node may run once per Job trigger, repeatedly, or once per level in
// a busy graph) would both be wasteful and — for SQLite's in-memory mode
// specifically — drop the database the moment the pool closes.
type sqlNode struct {
	nodeType   string
	driverName string
	mu         sync.Mutex
	conns      map[string]*sql.DB
}

func newSQLNode(nodeType, driverName string) *sqlNode {
	return &sqlNode{nodeType: nodeType, driverName: driverName, conns: map[string]*sql.DB{}}
}

func (n *sqlNode) connFor(dsn string) (*sql.DB, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	if db, ok := n.conns[dsn]; ok {
		return db, nil
	}
	db, err := sql.Open(n.driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", n.nodeType, err)
	}
	n.conns[dsn] = db
	return db, nil
}

func NewPostgres() *sqlNode { return newSQLNode("postgres", "postgres") }
func NewMySQL() *sqlNode    { return newSQLNode("mysql", "mysql") }
func NewSQLite() *sqlNode   { return newSQLNode("sqlite", "sqlite") }

func (n *sqlNode) Description() dataflow.NodeDescription {
	return dataflow.NodeDescription{Type: n.nodeType, Name: n.nodeType, Category: "Database",
		Description: fmt.Sprintf("Runs a query against a %s database. executeQuery returns one item per row; insert/update/delete return one item with the affected row count (field \"rows_affected\"). ", n.nodeType) +
			"Parameters: dsnSecretRef (string, required) — name of a configured dataflow secret holding the connection string. operation (string, required) — one of executeQuery/insert/update/delete. query (string, required) — the SQL, with ? placeholders. args (array, optional) — positional values for the placeholders.",
		Properties: []dataflow.NodeProperty{
			{Name: "dsnSecretRef", DisplayName: "Connection secret", Type: dataflow.PropSecretRef, Required: true,
				Description: "Name of a configured dataflow secret holding the connection string."},
			{Name: "operation", DisplayName: "Operation", Type: dataflow.PropOptions, Required: true, Options: []dataflow.NodePropertyOption{
				{Label: "Execute query", Value: "executeQuery"}, {Label: "Insert", Value: "insert"},
				{Label: "Update", Value: "update"}, {Label: "Delete", Value: "delete"},
			}},
			{Name: "query", DisplayName: "Query", Type: dataflow.PropText, Required: true, Placeholder: "SELECT * FROM table WHERE id = ?"},
			{Name: "args", DisplayName: "Arguments", Type: dataflow.PropJSON, Description: "Positional values for the query's ? placeholders."},
		}}
}

var validSQLOperations = map[string]bool{"executeQuery": true, "insert": true, "update": true, "delete": true}

func (n *sqlNode) ValidateParameters(params map[string]any) error {
	if dataflow.StringParam(params, "dsnSecretRef", "") == "" {
		return errors.New("dsnSecretRef is required")
	}
	operation := dataflow.StringParam(params, "operation", "")
	if !validSQLOperations[operation] {
		return fmt.Errorf("operation must be one of executeQuery/insert/update/delete, got %q", operation)
	}
	if dataflow.StringParam(params, "query", "") == "" {
		return errors.New("query is required")
	}
	return nil
}

// Execute runs params["query"] with params["args"] ([]any positional
// arguments) once per Run — like HTTPRequest, chain it after a node that
// produces one item per desired call if a per-item query is needed. A
// SELECT-shaped query (operation: executeQuery) returns one Item per row;
// insert/update/delete return one Item with the affected row count.
func (n *sqlNode) Execute(ctx context.Context, rt *dataflow.Runtime, input []dataflow.Item, params map[string]any) (dataflow.Output, error) {
	if rt == nil || rt.Secrets == nil {
		return dataflow.Output{}, errors.New("dataflow: no SecretResolver configured on Runtime")
	}
	dsn, err := rt.Secrets.Resolve(ctx, dataflow.StringParam(params, "dsnSecretRef", ""))
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("resolve dsn: %w", err)
	}

	db, err := n.connFor(dsn)
	if err != nil {
		return dataflow.Output{}, err
	}

	query := dataflow.StringParam(params, "query", "")
	args := paramArgs(params)

	if dataflow.StringParam(params, "operation", "") == "executeQuery" {
		return n.executeQuery(ctx, db, query, args)
	}
	res, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("exec: %w", err)
	}
	affected, _ := res.RowsAffected()
	return dataflow.Main([]dataflow.Item{{"rows_affected": affected}}), nil
}

// TestConnection verifies dsnSecretRef alone resolves and connects — it
// needs none of the other parameters (operation/query/args), so a node can
// be tested before the rest of its configuration is even filled in. See
// dataflow.TestableExecutor.
func (n *sqlNode) TestConnection(ctx context.Context, rt *dataflow.Runtime, params map[string]any) error {
	if rt == nil || rt.Secrets == nil {
		return errors.New("dataflow: no SecretResolver configured on Runtime")
	}
	dsn, err := rt.Secrets.Resolve(ctx, dataflow.StringParam(params, "dsnSecretRef", ""))
	if err != nil {
		return fmt.Errorf("resolve dsn: %w", err)
	}
	db, err := n.connFor(dsn)
	if err != nil {
		return err
	}
	return db.PingContext(ctx)
}

func (n *sqlNode) executeQuery(ctx context.Context, db *sql.DB, query string, args []any) (dataflow.Output, error) {
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("columns: %w", err)
	}

	var items []dataflow.Item
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return dataflow.Output{}, fmt.Errorf("scan row: %w", err)
		}
		item := make(dataflow.Item, len(columns))
		for i, col := range columns {
			item[col] = normalizeSQLValue(values[i])
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return dataflow.Output{}, fmt.Errorf("iterate rows: %w", err)
	}
	return dataflow.Main(items), nil
}

// normalizeSQLValue turns driver-returned []byte (how most drivers report
// TEXT/VARCHAR columns) into a plain string so items are JSON-serializable
// without a custom marshaler.
func normalizeSQLValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

func paramArgs(params map[string]any) []any {
	raw, _ := params["args"].([]any)
	return raw
}
