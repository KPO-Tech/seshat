package database

import "github.com/KPO-Tech/seshat-runtime/pkg/dataflow"

// Register adds every database node type in this package to registry.
func Register(registry *dataflow.Registry) {
	registry.Register("postgres", NewPostgres())
	registry.Register("mysql", NewMySQL())
	registry.Register("sqlite", NewSQLite())
	registry.Register("redis", NewRedis())
	registry.Register("mongodb", NewMongoDB())
	registry.Register("elasticsearch", NewElasticsearch())
}
