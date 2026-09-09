package database

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/KPO-Tech/seshat/pkg/dataflow"
)

// Elasticsearch is the "elasticsearch" node type. Elasticsearch's own API is
// already plain HTTP/JSON, so — matching m9m's own approach here, which is
// the right call for this one service — this talks to it directly rather
// than pulling in the official client SDK as a dependency.
//
// Parameters: baseURLSecretRef (required, resolves to e.g.
// "https://es.internal:9200"), index (required), operation
// (search/index/get/delete), id (for index/get/delete), document (for
// index), query (map[string]any, ES query DSL body, for search).
type Elasticsearch struct{ client *http.Client }

func NewElasticsearch() *Elasticsearch {
	return &Elasticsearch{client: &http.Client{Timeout: 30 * time.Second}}
}

func (n *Elasticsearch) Description() dataflow.NodeDescription {
	return dataflow.NodeDescription{Type: "elasticsearch", Name: "Elasticsearch", Category: "Database",
		Description: "Runs one operation against an Elasticsearch index. search returns one item per hit (with _id/_score merged with the source fields); index/get/delete return one item with the raw response. " +
			"Parameters: baseURLSecretRef (string, required) — name of a configured dataflow secret holding the base URL (e.g. \"https://es.internal:9200\"). index (string, required). operation (string, required) — search/index/get/delete. id (string, required for get/delete, optional for index). document (object, for index). query (object, for search — an Elasticsearch query DSL clause).",
		Properties: []dataflow.NodeProperty{
			{Name: "baseURLSecretRef", DisplayName: "Base URL secret", Type: dataflow.PropSecretRef, Required: true,
				Description: "Name of a configured dataflow secret holding the base URL, e.g. \"https://es.internal:9200\"."},
			{Name: "index", DisplayName: "Index", Type: dataflow.PropString, Required: true},
			{Name: "operation", DisplayName: "Operation", Type: dataflow.PropOptions, Required: true, Options: []dataflow.NodePropertyOption{
				{Label: "Search", Value: "search"}, {Label: "Index", Value: "index"},
				{Label: "Get", Value: "get"}, {Label: "Delete", Value: "delete"},
			}},
			{Name: "id", DisplayName: "Document ID", Type: dataflow.PropString, Description: "Required for get/delete, optional for index.",
				DisplayIf: &dataflow.DisplayCondition{Field: "operation", Equals: []string{"get", "delete", "index"}}},
			{Name: "document", DisplayName: "Document", Type: dataflow.PropJSON,
				DisplayIf: &dataflow.DisplayCondition{Field: "operation", Equals: []string{"index"}}},
			{Name: "query", DisplayName: "Query", Type: dataflow.PropJSON, Description: "Elasticsearch query DSL clause.",
				DisplayIf: &dataflow.DisplayCondition{Field: "operation", Equals: []string{"search"}}},
		}}
}

var validESOps = map[string]bool{"search": true, "index": true, "get": true, "delete": true}

func (n *Elasticsearch) ValidateParameters(params map[string]any) error {
	if dataflow.StringParam(params, "baseURLSecretRef", "") == "" {
		return errors.New("baseURLSecretRef is required")
	}
	if dataflow.StringParam(params, "index", "") == "" {
		return errors.New("index is required")
	}
	operation := dataflow.StringParam(params, "operation", "")
	if !validESOps[operation] {
		return errors.New("operation must be one of search/index/get/delete")
	}
	if (operation == "get" || operation == "delete") && dataflow.StringParam(params, "id", "") == "" {
		return fmt.Errorf("id is required for operation %q", operation)
	}
	return nil
}

// TestConnection verifies baseURLSecretRef alone resolves and reaches a real
// Elasticsearch cluster — it needs none of the other parameters
// (index/operation/...). A GET on the cluster root is the cheapest request
// every Elasticsearch deployment answers regardless of index. See
// dataflow.TestableExecutor.
func (n *Elasticsearch) TestConnection(ctx context.Context, rt *dataflow.Runtime, params map[string]any) error {
	if rt == nil || rt.Secrets == nil {
		return errors.New("dataflow: no SecretResolver configured on Runtime")
	}
	baseURL, err := rt.Secrets.Resolve(ctx, dataflow.StringParam(params, "baseURLSecretRef", ""))
	if err != nil {
		return fmt.Errorf("resolve base url: %w", err)
	}
	baseURL = strings.TrimRight(baseURL, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/", nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("elasticsearch returned HTTP %d", resp.StatusCode)
	}
	return nil
}

func (n *Elasticsearch) Execute(ctx context.Context, rt *dataflow.Runtime, input []dataflow.Item, params map[string]any) (dataflow.Output, error) {
	if rt == nil || rt.Secrets == nil {
		return dataflow.Output{}, errors.New("dataflow: no SecretResolver configured on Runtime")
	}
	baseURL, err := rt.Secrets.Resolve(ctx, dataflow.StringParam(params, "baseURLSecretRef", ""))
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("resolve base url: %w", err)
	}
	baseURL = strings.TrimRight(baseURL, "/")
	index := dataflow.StringParam(params, "index", "")
	id := dataflow.StringParam(params, "id", "")

	var method, path string
	var body any
	switch dataflow.StringParam(params, "operation", "") {
	case "search":
		method, path = http.MethodPost, fmt.Sprintf("/%s/_search", index)
		body = map[string]any{"query": params["query"]}
	case "index":
		if id != "" {
			method, path = http.MethodPut, fmt.Sprintf("/%s/_doc/%s", index, id)
		} else {
			method, path = http.MethodPost, fmt.Sprintf("/%s/_doc", index)
		}
		body = params["document"]
	case "get":
		method, path = http.MethodGet, fmt.Sprintf("/%s/_doc/%s", index, id)
	case "delete":
		method, path = http.MethodDelete, fmt.Sprintf("/%s/_doc/%s", index, id)
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return dataflow.Output{}, fmt.Errorf("encode body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reader)
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return dataflow.Output{}, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return dataflow.Output{}, fmt.Errorf("elasticsearch returned %d: %s", resp.StatusCode, string(respBody))
	}

	var parsed map[string]any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return dataflow.Main([]dataflow.Item{{"raw": string(respBody)}}), nil
	}

	if dataflow.StringParam(params, "operation", "") == "search" {
		return dataflow.Main(esSearchHits(parsed)), nil
	}
	return dataflow.Main([]dataflow.Item{dataflow.Item(parsed)}), nil
}

func esSearchHits(response map[string]any) []dataflow.Item {
	hitsWrapper, _ := response["hits"].(map[string]any)
	hits, _ := hitsWrapper["hits"].([]any)
	items := make([]dataflow.Item, 0, len(hits))
	for _, h := range hits {
		hit, ok := h.(map[string]any)
		if !ok {
			continue
		}
		item := dataflow.Item{"_id": hit["_id"], "_score": hit["_score"]}
		if source, ok := hit["_source"].(map[string]any); ok {
			for k, v := range source {
				item[k] = v
			}
		}
		items = append(items, item)
	}
	return items
}
