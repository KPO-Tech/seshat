// Package titlemodel names sessions with a tiny language model that runs on the user's own machine.
//
// A session needs a short title, and asking the chat provider for it costs a request, needs the network and shows
// the first message to the provider again. A 0.5-billion-parameter instruct model answers in about a second on a CPU,
// offline and for free. The package downloads two things on request, checks both against sums fixed in this file,
// and runs them: the llama.cpp server (llama-server) and a small GGUF model. Nothing is downloaded or started unless
// the user installed the model (`seshat title-model install`), and when it is missing or fails, the engine falls
// back to the provider as before.
package titlemodel

import "fmt"

// LlamaTag is the llama.cpp release the server comes from. Raising it means checking the new archives and updating
// every sum below.
const LlamaTag = "b11342"

// LlamaBaseURL is where the archives are published.
const LlamaBaseURL = "https://github.com/ggml-org/llama.cpp/releases/download/" + LlamaTag

// HFBaseURL is where the model files are published.
const HFBaseURL = "https://huggingface.co"

// Archive is a llama.cpp CPU build for one platform.
type Archive struct {
	Name   string
	SHA256 string
	Size   int64
}

// archives are the CPU builds of LlamaTag, by "goos-goarch". The sums come from the digests GitHub publishes with the
// release assets.
var archives = map[string]Archive{
	"windows-amd64": {"llama-" + LlamaTag + "-bin-win-cpu-x64.zip", "cc6f3ac938988f6ed4ce204b119d6d1ffe59342cb3157adef20e8132681acd8d", 19274166},
	"windows-arm64": {"llama-" + LlamaTag + "-bin-win-cpu-arm64.zip", "1dcb2c3af748c5db93b0c603884ed911b012a4d059f5887bbe717b08e1c47e33", 12117198},
	"linux-amd64":   {"llama-" + LlamaTag + "-bin-ubuntu-x64.tar.gz", "7c8f7eb14cfb4a8dceb1f6cc6220ea387a1940f0c95cab76ca63ac35d2934fd4", 17552025},
	"linux-arm64":   {"llama-" + LlamaTag + "-bin-ubuntu-arm64.tar.gz", "6f5f88d9e105230c32b13ad9d8bba9611ab29ee27532717ba6bb5bb4120f3c85", 13587967},
	"darwin-amd64":  {"llama-" + LlamaTag + "-bin-macos-x64.tar.gz", "f5603510bcf2374d02ee1bd5ab1d307b60dec9f9c03fa67dd88a6c049089e49a", 11383367},
	"darwin-arm64":  {"llama-" + LlamaTag + "-bin-macos-arm64.tar.gz", "1050318ed5fb941a1b4c1c603f6c3c7fd3f065b56af3b8cfab39aee70f3ad076", 11828239},
}

// ArchiveFor returns the llama.cpp build for a platform.
func ArchiveFor(goos, goarch string) (Archive, bool) {
	a, ok := archives[goos+"-"+goarch]
	return a, ok
}

// Model is a GGUF instruct model small enough to name a session in about a second on a CPU. Only models that answer
// directly belong here: a reasoning model spends its output budget thinking and never returns a title.
type Model struct {
	ID          string
	Label       string
	Repo        string
	File        string
	SHA256      string
	Size        int64
	Description string
}

// DefaultModelID is the model installed when none is chosen.
const DefaultModelID = "qwen2.5-0.5b"

// Models is the catalog, the same models the desktop offers.
var Models = []Model{
	{
		ID: "qwen2.5-0.5b", Label: "Qwen2.5 0.5B Instruct", Repo: "Qwen/Qwen2.5-0.5B-Instruct-GGUF",
		File: "qwen2.5-0.5b-instruct-q4_k_m.gguf", SHA256: "74a4da8c9fdbcd15bd1f6d01d621410d31c6fc00986f5eb687824e7b93d7a9db",
		Size: 491400032, Description: "Default. Small, multilingual, Apache-2.0.",
	},
	{
		ID: "gemma-3-270m", Label: "Gemma 3 270M Instruct", Repo: "unsloth/gemma-3-270m-it-GGUF",
		File: "gemma-3-270m-it-Q4_K_M.gguf", SHA256: "b1baabd6b729e4041822220d3e648e00d99cac5df86b10dffb77bcccf0688e39",
		Size: 253115424, Description: "Smallest option. Faster, rougher titles. Gemma terms of use apply.",
	},
}

// FindModel returns the catalog entry with the given id.
func FindModel(id string) (Model, error) {
	for _, m := range Models {
		if m.ID == id {
			return m, nil
		}
	}
	return Model{}, fmt.Errorf("unknown title model %q", id)
}
