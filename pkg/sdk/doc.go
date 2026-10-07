// Package sdk embeds the Seshat agent runtime in a Go program.
//
// A Client holds the configuration of the runtime (the provider and the model, the tools, the permission rules, the storage) and
// creates Sessions; a Session runs the turns of a conversation, calling the model and the tools it asks for, and reports what happens
// through callbacks and event queues.
//
// The documentation is on the website: the Go SDK reference is at https://seshat-ai.com/en/docs/sdk/go-sdk, the installation guide
// at https://seshat-ai.com/en/docs/getting-started/installation and the other pages (concepts, the gRPC API, the CLI) at
// https://seshat-ai.com/en/docs.
package sdk
