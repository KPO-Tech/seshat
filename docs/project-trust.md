# Project trust (terminal UI)

The terminal UI reads its configuration from the user's own files (in the runtime root, for example `~/.config/seshat/seshat.json`) and from the **project**: a `.seshat.json` or `seshat.json` found from the working directory up to the git root, and `.seshat/seshat.json`. A project is not always the user's own: it can be a repository that was just cloned, and its files can say anything.

Some of what they say runs something or sends something somewhere, so it is **ignored until the project is trusted**:

| Section | Why |
|---|---|
| `mcp` / `mcpServers` | an MCP server in stdio mode is a command line started when Seshat opens |
| `hooks` | shell commands that run on tool events |
| `lsp` | language servers are programs |
| `providers` | a base URL decides who receives the API key; `$(...)` in a value is evaluated by a shell |
| `permissions` | `allowed_tools` removes the permission questions |
| `image_generation`, `text_to_speech`, `speech_to_text` | endpoints and keys |
| `options.context_paths`, `options.skills_paths` | can take any file of the machine to the model provider, or add instructions |

The rest (models, display options, disabled tools...) applies as before. The user's own configuration is never restricted.

When something is ignored, the interface shows a warning at startup and the log says which file and which sections.

## Trusting a project

```
seshat trust              # trust the configuration files of this project as they are now
seshat trust --status     # list the files, whether they are trusted, and what they hold
seshat trust --untrust    # withdraw the trust
```

The trust is recorded for the **content** of each file (its SHA-256) in `trusted_projects.json` in the runtime root: a file that changes, because a pull brought a new version, has to be trusted again. A setting that the user changes through the interface keeps a file trusted if it was, and trusts a new file; it does not trust a file that came with the repository and holds restricted sections.

Trust a project you wrote or read, not one you only cloned.

## Related: the `.env` file of the working directory

`pkg/config` also reads a `.env` in the working directory. Its entries are ignored, with a warning, when their value holds a shell substitution (`$(...)` or a backquote) or their name decides what is run or where Seshat looks for its configuration (`PATH`, `SHELL`, `LD_*`, `GIT_*`, `NODE_OPTIONS`, `SESHAT_RUNTIME_ROOT`, `SESHAT_BASH_*`...). Keys, models and URLs are still read from it. When a `*_BASE_URL` (or `RAG_EMBEDDING_URL`, `RAG_RERANK_URL`, `SESHAT_S3_ENDPOINT`, `SEARXNG_BASE_URL`) points outside this machine (not `localhost`, loopback or a private address), Seshat applies it and prints a warning naming the server that will receive the requests with their API key, because a `.env` that came with a cloned repository can use it to collect keys.
