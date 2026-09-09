package database

import (
	"context"
	"errors"
	"fmt"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/KPO-Tech/seshat/pkg/dataflow"
)

// MongoDB is the "mongodb" node type. Unlike m9m's MongoDB node (which only
// works against MongoDB Atlas's Data API — a cloud-only REST proxy, not
// available for a plain self-hosted MongoDB), this uses the official native
// driver so it works against any MongoDB deployment.
//
// Parameters: uriSecretRef (required), database, collection (required),
// operation (find/insertOne/updateOne/deleteOne), filter (map[string]any,
// query for find/updateOne/deleteOne), document (map[string]any, for
// insertOne), update (map[string]any, for updateOne — wrapped in $set).
type MongoDB struct{}

func NewMongoDB() MongoDB { return MongoDB{} }

func (MongoDB) Description() dataflow.NodeDescription {
	return dataflow.NodeDescription{Type: "mongodb", Name: "MongoDB", Category: "Database",
		Description: "Runs one operation against a MongoDB collection. find returns one item per matched document; insertOne/updateOne/deleteOne return one item summarizing the result. " +
			"Parameters: uriSecretRef (string, required) — name of a configured dataflow secret holding the connection URI. database, collection (string, required). operation (string, required) — find/insertOne/updateOne/deleteOne. filter (object, for find/updateOne/deleteOne). document (object, for insertOne). update (object, for updateOne — wrapped in $set).",
		Properties: []dataflow.NodeProperty{
			{Name: "uriSecretRef", DisplayName: "Connection secret", Type: dataflow.PropSecretRef, Required: true,
				Description: "Name of a configured dataflow secret holding the connection URI."},
			{Name: "database", DisplayName: "Database", Type: dataflow.PropString, Required: true},
			{Name: "collection", DisplayName: "Collection", Type: dataflow.PropString, Required: true},
			{Name: "operation", DisplayName: "Operation", Type: dataflow.PropOptions, Required: true, Options: []dataflow.NodePropertyOption{
				{Label: "Find", Value: "find"}, {Label: "Insert one", Value: "insertOne"},
				{Label: "Update one", Value: "updateOne"}, {Label: "Delete one", Value: "deleteOne"},
			}},
			{Name: "filter", DisplayName: "Filter", Type: dataflow.PropJSON,
				DisplayIf: &dataflow.DisplayCondition{Field: "operation", Equals: []string{"find", "updateOne", "deleteOne"}}},
			{Name: "document", DisplayName: "Document", Type: dataflow.PropJSON,
				DisplayIf: &dataflow.DisplayCondition{Field: "operation", Equals: []string{"insertOne"}}},
			{Name: "update", DisplayName: "Update", Type: dataflow.PropJSON, Description: "Wrapped in $set.",
				DisplayIf: &dataflow.DisplayCondition{Field: "operation", Equals: []string{"updateOne"}}},
		}}
}

var validMongoOps = map[string]bool{"find": true, "insertOne": true, "updateOne": true, "deleteOne": true}

func (MongoDB) ValidateParameters(params map[string]any) error {
	if dataflow.StringParam(params, "uriSecretRef", "") == "" {
		return errors.New("uriSecretRef is required")
	}
	if dataflow.StringParam(params, "database", "") == "" || dataflow.StringParam(params, "collection", "") == "" {
		return errors.New("database and collection are required")
	}
	if !validMongoOps[dataflow.StringParam(params, "operation", "")] {
		return errors.New("operation must be one of find/insertOne/updateOne/deleteOne")
	}
	return nil
}

// TestConnection verifies uriSecretRef alone resolves and connects — it
// needs none of the other parameters (database/collection/operation/...).
// mongo.Connect itself is lazy (it doesn't verify connectivity), so this
// explicitly Pings — see dataflow.TestableExecutor.
func (MongoDB) TestConnection(ctx context.Context, rt *dataflow.Runtime, params map[string]any) error {
	if rt == nil || rt.Secrets == nil {
		return errors.New("dataflow: no SecretResolver configured on Runtime")
	}
	uri, err := rt.Secrets.Resolve(ctx, dataflow.StringParam(params, "uriSecretRef", ""))
	if err != nil {
		return fmt.Errorf("resolve uri: %w", err)
	}
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer client.Disconnect(ctx)
	return client.Ping(ctx, nil)
}

func (MongoDB) Execute(ctx context.Context, rt *dataflow.Runtime, input []dataflow.Item, params map[string]any) (dataflow.Output, error) {
	if rt == nil || rt.Secrets == nil {
		return dataflow.Output{}, errors.New("dataflow: no SecretResolver configured on Runtime")
	}
	uri, err := rt.Secrets.Resolve(ctx, dataflow.StringParam(params, "uriSecretRef", ""))
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("resolve uri: %w", err)
	}
	client, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("connect: %w", err)
	}
	defer client.Disconnect(ctx)

	coll := client.Database(dataflow.StringParam(params, "database", "")).Collection(dataflow.StringParam(params, "collection", ""))
	filter := toBsonM(params["filter"])

	switch dataflow.StringParam(params, "operation", "") {
	case "find":
		cursor, err := coll.Find(ctx, filter)
		if err != nil {
			return dataflow.Output{}, err
		}
		defer cursor.Close(ctx)
		var items []dataflow.Item
		for cursor.Next(ctx) {
			var doc bson.M
			if err := cursor.Decode(&doc); err != nil {
				return dataflow.Output{}, fmt.Errorf("decode: %w", err)
			}
			items = append(items, dataflow.Item(doc))
		}
		return dataflow.Main(items), cursor.Err()
	case "insertOne":
		doc := toBsonM(params["document"])
		res, err := coll.InsertOne(ctx, doc)
		if err != nil {
			return dataflow.Output{}, err
		}
		return dataflow.Main([]dataflow.Item{{"inserted_id": fmt.Sprintf("%v", res.InsertedID)}}), nil
	case "updateOne":
		update := bson.M{"$set": toBsonM(params["update"])}
		res, err := coll.UpdateOne(ctx, filter, update)
		if err != nil {
			return dataflow.Output{}, err
		}
		return dataflow.Main([]dataflow.Item{{"matched": res.MatchedCount, "modified": res.ModifiedCount}}), nil
	case "deleteOne":
		res, err := coll.DeleteOne(ctx, filter)
		if err != nil {
			return dataflow.Output{}, err
		}
		return dataflow.Main([]dataflow.Item{{"deleted": res.DeletedCount}}), nil
	}
	return dataflow.Output{}, fmt.Errorf("unsupported operation %q", dataflow.StringParam(params, "operation", ""))
}

func toBsonM(v any) bson.M {
	m, ok := v.(map[string]any)
	if !ok {
		return bson.M{}
	}
	return bson.M(m)
}
