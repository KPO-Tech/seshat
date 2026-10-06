# Licensing and provenance

Seshat is published under the [Apache License 2.0](../LICENSE). One part of the repository has another origin, recorded in [NOTICE](../NOTICE).

## The terminal UI (`internal/seshattui`)

The terminal UI was built from the terminal interface of [Crush](https://github.com/charmbracelet/crush) (Charmbracelet, Inc.), which is published under the **Functional Source License 1.1, MIT Future License** (FSL-1.1-MIT):

- use, copy, modification and redistribution are allowed for any purpose **except a Competing Use**: making the software available to others in a commercial product or service that substitutes for Crush, or for another product or service of Charmbracelet that uses it;
- each version of Crush becomes available under the MIT licence two years after it was published;
- the licence text is at <https://github.com/charmbracelet/crush/blob/main/LICENSE.md>.

What this means here:

- The files of `internal/seshattui` that come from Crush stay subject to the FSL-1.1-MIT in addition to the Apache-2.0 terms of the rest of the repository. Apache-2.0 does not replace it for them.
- The engine (`pkg/`, the other `internal/` packages, `cmd/grpc`) is not part of this: it does not use that code.
- Someone who redistributes the terminal UI inside a commercial coding-agent product should read the Crush licence first, or remove `internal/seshattui` and use the engine through the SDK or gRPC.

## Open question

Which files of `internal/seshattui` are copied, and which were written for Seshat, is not recorded file by file. Until that is done, the whole directory is treated as subject to the Crush licence. A clean separation (rewriting the copied parts, or asking Charmbracelet for a licence) would let the directory be Apache-2.0 like the rest.
