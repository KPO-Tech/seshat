# Licensing and provenance

Seshat is published under the [Apache License 2.0](../LICENSE). One part of the repository has another origin, recorded in [NOTICE](../NOTICE).

## The terminal UI (`tui/internal/seshattui`)

The terminal UI was built from the terminal interface of [Crush](https://github.com/charmbracelet/crush) (Charmbracelet, Inc.), which is published under the **Functional Source License 1.1, MIT Future License** (FSL-1.1-MIT):

- use, copy, modification and redistribution are allowed for any purpose **except a Competing Use**: making the software available to others in a commercial product or service that substitutes for Crush, or for another product or service of Charmbracelet that uses it;
- each version of Crush becomes available under the MIT licence two years after it was published;
- the licence text is at <https://github.com/charmbracelet/crush/blob/main/LICENSE.md>.

What this means here:

- The files of `tui/internal/seshattui` that come from Crush stay subject to the FSL-1.1-MIT in addition to the Apache-2.0 terms of the rest of the repository. Apache-2.0 does not replace it for them.
- The engine (`pkg/`, the other `internal/` packages, `cmd/grpc`) is not part of this: it does not use that code.
- Someone who redistributes the terminal UI inside a commercial coding-agent product should read the Crush licence first, or remove `tui/internal/seshattui` and use the engine through the SDK or gRPC.

## What this means for selling Seshat

This is a description of the licence text, not legal advice; have a lawyer read the Crush licence before a commercial release.

- **The engine and the gRPC server are not affected.** `pkg/`, `cmd/grpc` and every `internal/` package are Apache-2.0 and live in the engine module, which does not import the terminal interface: the terminal command and its interface are the `tui` module (`tui/`), and `internal/archtest/tui_isolation_test.go` fails if the engine starts importing it.
- **SeshatOS and SeshatCloud are not affected**: they use the engine through `pkg/*` and do not contain the terminal interface.
- **The `seshat` terminal binary (the `tui` module) is the exposed part.** It contains code from Crush. FSL allows using, modifying and redistributing it for any purpose except a *Competing Use*: making it available to others in a commercial product or service that substitutes for Crush or offers substantially similar functionality. Crush is a terminal coding agent and so is `seshat`, so selling or hosting the `seshat` terminal binary as a paid product is the case the licence forbids. Using it yourself, in consulting work for clients (running it for them rather than selling it as a product), giving it away free of charge, and contributing to it are not what the Competing Use clause describes.
- **It expires.** Each Crush version becomes MIT two years after it was published, so the copied code that is two years old is already free to use commercially. A new copy taken today is not.
- **Rewriting does not clean it by itself.** A port to another language (Rust, for example) of the Crush-derived files is still a derivative work. Only a clean-room rewrite, written from a specification of the behavior without reading or translating the copied code, is free of the licence.

Until the terminal interface is rewritten, do not present the `seshat` binary as a commercial product; present the engine, SeshatOS and SeshatCloud.

## Open question

Which files of `tui/internal/seshattui` are copied, and which were written for Seshat, is not recorded file by file. Until that is done, the whole directory is treated as subject to the Crush licence. A clean separation (rewriting the copied parts, or asking Charmbracelet for a licence) would let the directory be Apache-2.0 like the rest.
