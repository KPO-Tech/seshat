package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"

	"github.com/KPO-Tech/seshat/pkg/dataflow"
)

// Redis is the "redis" node type. Unlike m9m's Redis node (an HTTP call to
// a REST-to-Redis gateway like Upstash — a real limitation for a plain
// self-hosted Redis, which exposes no such gateway by default), this uses
// go-redis's native client so it works against any standard Redis instance.
//
// Parameters: addrSecretRef (required, resolves to "host:port"),
// passwordSecretRef (optional), operation (get/set/del/keys/hget/hset/lpush/
// rpush), key, value (for set/hset/lpush/rpush), field (for hget/hset).
type Redis struct{}

func NewRedis() Redis { return Redis{} }

func (Redis) Description() dataflow.NodeDescription {
	return dataflow.NodeDescription{Type: "redis", Name: "Redis", Category: "Database",
		Description: "Runs one Redis command, returning one item with the result. " +
			"Parameters: addrSecretRef (string, required) — name of a configured dataflow secret holding \"host:port\". passwordSecretRef (string, optional). operation (string, required) — get/set/del/keys/hget/hset/lpush/rpush. key (string, required). value (string, for set/hset/lpush/rpush). field (string, for hget/hset).",
		Properties: []dataflow.NodeProperty{
			{Name: "addrSecretRef", DisplayName: "Address secret", Type: dataflow.PropSecretRef, Required: true,
				Description: "Name of a configured dataflow secret holding \"host:port\"."},
			{Name: "passwordSecretRef", DisplayName: "Password secret", Type: dataflow.PropSecretRef},
			{Name: "operation", DisplayName: "Operation", Type: dataflow.PropOptions, Required: true, Options: []dataflow.NodePropertyOption{
				{Label: "GET", Value: "get"}, {Label: "SET", Value: "set"}, {Label: "DEL", Value: "del"}, {Label: "KEYS", Value: "keys"},
				{Label: "HGET", Value: "hget"}, {Label: "HSET", Value: "hset"}, {Label: "LPUSH", Value: "lpush"}, {Label: "RPUSH", Value: "rpush"},
			}},
			{Name: "key", DisplayName: "Key", Type: dataflow.PropString, Required: true},
			{Name: "value", DisplayName: "Value", Type: dataflow.PropString,
				DisplayIf: &dataflow.DisplayCondition{Field: "operation", Equals: []string{"set", "hset", "lpush", "rpush"}}},
			{Name: "field", DisplayName: "Field", Type: dataflow.PropString,
				DisplayIf: &dataflow.DisplayCondition{Field: "operation", Equals: []string{"hget", "hset"}}},
		}}
}

var validRedisOps = map[string]bool{"get": true, "set": true, "del": true, "keys": true, "hget": true, "hset": true, "lpush": true, "rpush": true}

func (Redis) ValidateParameters(params map[string]any) error {
	if dataflow.StringParam(params, "addrSecretRef", "") == "" {
		return errors.New("addrSecretRef is required")
	}
	if !validRedisOps[dataflow.StringParam(params, "operation", "")] {
		return errors.New("operation must be one of get/set/del/keys/hget/hset/lpush/rpush")
	}
	if dataflow.StringParam(params, "key", "") == "" {
		return errors.New("key is required")
	}
	return nil
}

// TestConnection verifies addrSecretRef (+ passwordSecretRef, if set) alone
// resolve and connect — see dataflow.TestableExecutor.
func (Redis) TestConnection(ctx context.Context, rt *dataflow.Runtime, params map[string]any) error {
	if rt == nil || rt.Secrets == nil {
		return errors.New("dataflow: no SecretResolver configured on Runtime")
	}
	addr, err := rt.Secrets.Resolve(ctx, dataflow.StringParam(params, "addrSecretRef", ""))
	if err != nil {
		return fmt.Errorf("resolve addr: %w", err)
	}
	opts := &redis.Options{Addr: addr}
	if ref := dataflow.StringParam(params, "passwordSecretRef", ""); ref != "" {
		password, err := rt.Secrets.Resolve(ctx, ref)
		if err != nil {
			return fmt.Errorf("resolve password: %w", err)
		}
		opts.Password = password
	}
	client := redis.NewClient(opts)
	defer client.Close()
	return client.Ping(ctx).Err()
}

func (Redis) Execute(ctx context.Context, rt *dataflow.Runtime, input []dataflow.Item, params map[string]any) (dataflow.Output, error) {
	if rt == nil || rt.Secrets == nil {
		return dataflow.Output{}, errors.New("dataflow: no SecretResolver configured on Runtime")
	}
	addr, err := rt.Secrets.Resolve(ctx, dataflow.StringParam(params, "addrSecretRef", ""))
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("resolve addr: %w", err)
	}
	opts := &redis.Options{Addr: addr}
	if ref := dataflow.StringParam(params, "passwordSecretRef", ""); ref != "" {
		password, err := rt.Secrets.Resolve(ctx, ref)
		if err != nil {
			return dataflow.Output{}, fmt.Errorf("resolve password: %w", err)
		}
		opts.Password = password
	}
	client := redis.NewClient(opts)
	defer client.Close()

	key := dataflow.StringParam(params, "key", "")
	value := dataflow.StringParam(params, "value", "")
	field := dataflow.StringParam(params, "field", "")

	switch dataflow.StringParam(params, "operation", "") {
	case "get":
		v, err := client.Get(ctx, key).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return dataflow.Output{}, err
		}
		return dataflow.Main([]dataflow.Item{{"key": key, "value": v, "found": !errors.Is(err, redis.Nil)}}), nil
	case "set":
		if err := client.Set(ctx, key, value, 0).Err(); err != nil {
			return dataflow.Output{}, err
		}
		return dataflow.Main([]dataflow.Item{{"key": key, "set": true}}), nil
	case "del":
		n, err := client.Del(ctx, key).Result()
		if err != nil {
			return dataflow.Output{}, err
		}
		return dataflow.Main([]dataflow.Item{{"deleted": n}}), nil
	case "keys":
		matches, err := client.Keys(ctx, key).Result()
		if err != nil {
			return dataflow.Output{}, err
		}
		items := make([]dataflow.Item, len(matches))
		for i, k := range matches {
			items[i] = dataflow.Item{"key": k}
		}
		return dataflow.Main(items), nil
	case "hget":
		v, err := client.HGet(ctx, key, field).Result()
		if err != nil && !errors.Is(err, redis.Nil) {
			return dataflow.Output{}, err
		}
		return dataflow.Main([]dataflow.Item{{"key": key, "field": field, "value": v, "found": !errors.Is(err, redis.Nil)}}), nil
	case "hset":
		if err := client.HSet(ctx, key, field, value).Err(); err != nil {
			return dataflow.Output{}, err
		}
		return dataflow.Main([]dataflow.Item{{"key": key, "field": field, "set": true}}), nil
	case "lpush":
		n, err := client.LPush(ctx, key, value).Result()
		if err != nil {
			return dataflow.Output{}, err
		}
		return dataflow.Main([]dataflow.Item{{"key": key, "length": n}}), nil
	case "rpush":
		n, err := client.RPush(ctx, key, value).Result()
		if err != nil {
			return dataflow.Output{}, err
		}
		return dataflow.Main([]dataflow.Item{{"key": key, "length": n}}), nil
	}
	return dataflow.Output{}, fmt.Errorf("unsupported operation %q", dataflow.StringParam(params, "operation", ""))
}
