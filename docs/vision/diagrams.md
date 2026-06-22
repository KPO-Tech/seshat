# Seshat — Architecture Diagrams

Rendered images are in `docs/images/`. Remaining sequence diagrams are in Mermaid (render with [mermaid.live](https://mermaid.live) or VS Code extension).

---

## 1. Complete project overview

The full system at target state: engine, product layer, SDK ecosystem, and consumers.

<p align="center">
  <img src="../images/ideal_vision.png" alt="Complete project overview" width="900">
</p>

---

## 2. seshat internal architecture

Package-level dependency map inside the engine.

<p align="center">
  <img src="../images/architecture_seshat.png" alt="seshat internal architecture" width="900">
</p>

---

## 3. Agent execution loop — state machine

One turn: from user prompt to final response.

<p align="center">
  <img src="../images/agent_loop.png" alt="Agent execution loop state machine" width="800">
</p>

---

## 4. Provider routing

Retry, circuit breaker, and fallback chain.

<p align="center">
  <img src="../images/provider_routing.png" alt="Provider routing" width="800">
</p>

---

## 5. Permission pipeline

How each tool call is evaluated before execution.

<p align="center">
  <img src="../images/permission_pipeline.png" alt="Permission pipeline" width="700">
</p>

---

## 6. Session and memory lifecycle

From session creation to cross-session memory persistence.

```mermaid
sequenceDiagram
    participant App as Application
    participant Client as sdk.Client
    participant S as sdk.Session
    participant EngineLoop as engine.Loop
    participant Memory as memory.Manager
    participant Store as SessionStore

    App->>Client: NewClient(config)
    Client->>Memory: load project and user memory
    Client->>Store: open session store

    App->>Client: CreateSession(ctx)
    Client->>S: new session
    S->>Store: SaveSession(metadata)
    S->>Memory: inject memory context into prompt

    loop Each turn
        App->>S: SubmitMessage(prompt)
        S->>EngineLoop: Run(turn)
        EngineLoop->>EngineLoop: build prompt
        EngineLoop->>EngineLoop: call model
        EngineLoop->>EngineLoop: execute tools
        EngineLoop-->>S: RuntimeEvents streaming
        S-->>App: ResponseChunk callbacks
        EngineLoop->>Store: AppendTranscriptEntries
        S-->>App: SessionResponse final
    end

    Note over EngineLoop,Store: Auto-compact if context >= 85 percent

    EngineLoop->>Store: ReplaceTranscript compacted
    Store->>Store: SaveCheckpoint

    App->>S: Close()
    S->>Store: SaveSessionState final
    S->>Memory: save cross-session learnings

    App->>Client: LoadSession(savedID)
    Client->>Store: RestoreSessionState
    Store-->>Client: metadata and messages
    Client->>S: NewSessionFromState
```

---

## 7. Level 1 → Level 2 → Level 3 progression

How the runtime capability grows across levels.

<p align="center">
  <img src="../images/runtime_progession.png" alt="Level 1 to Level 3 progression" width="900">
</p>

---

## 8. Sub-agent execution — Level 2

One-shot and parallel delegation patterns.

```mermaid
sequenceDiagram
    participant Orch as Orchestrator session
    participant Engine as seshat
    participant SA1 as Sub-agent 1\n(isolated session)
    participant SA2 as Sub-agent 2\n(isolated session)
    participant SA3 as Sub-agent 3\n(isolated session)

    Note over Orch: parallel spawn pattern

    Orch->>Engine: spawn_agent("research competitor A", tools=[web_search, read], budget=50K)
    Orch->>Engine: spawn_agent("research competitor B", tools=[web_search, read], budget=50K)
    Orch->>Engine: spawn_agent("analyse our pricing",    tools=[read, bash],       budget=30K)

    par all three run concurrently
        Engine->>SA1: new session + skills + budget
        SA1-->>Engine: progress events
        SA1-->>Engine: result

    and
        Engine->>SA2: new session + skills + budget
        SA2-->>Engine: progress events
        SA2-->>Engine: result

    and
        Engine->>SA3: new session + skills + budget
        SA3-->>Engine: progress events
        SA3-->>Engine: result
    end

    Engine-->>Orch: runtime events (agent.spawn, agent.wait, agent.end)

    Orch->>Engine: wait_agent([all three IDs])
    Engine-->>Orch: collected results as tool results

    Note over Orch: orchestrator now has all results\nand continues its own reasoning
    Orch->>Engine: next model call (integrate results → write report)
```

---

## 9. Team runtime — Level 3

How agents coordinate via mailbox and task board.

```mermaid
sequenceDiagram
    participant Mission as Mission\n(trigger)
    participant CEO as CEO agent
    participant CTO as CTO agent
    participant Dev as Dev agent
    participant QA as QA agent
    participant Board as Task board\n(shared)
    participant Mail as Mailboxes

    Mission->>CEO: new mission posted

    CEO->>CEO: read mission, plan
    CEO->>Board: post_task("Architecture design", assign=CTO)
    CEO->>Board: post_task("Implementation",      assign=Dev)
    CEO->>Board: post_task("QA review",            assign=QA)

    Board-->>CTO: notification (task available)
    CTO->>Board: claim_task("Architecture design")
    CTO->>CTO: research + design
    CTO->>Mail: send_mail(to=Dev, "Here is the architecture spec")
    CTO->>Board: complete_task("Architecture design")

    Board-->>Dev: task ready
    Mail-->>Dev: mail from CTO
    Dev->>Board: claim_task("Implementation")
    Dev->>Dev: read spec, implement
    Dev->>Dev: spawn sub-agents for unit tests (Level 2)
    Dev->>Board: complete_task("Implementation")

    Board-->>QA: task ready
    QA->>Board: claim_task("QA review")
    QA->>QA: review + test
    QA->>Mail: send_mail(to=Dev, "2 bugs found: …")

    Mail-->>Dev: bug report
    Dev->>Dev: fix bugs
    Dev->>Mail: send_mail(to=QA, "Fixed, please re-review")

    QA->>QA: re-review
    QA->>Board: complete_task("QA review")

    Board-->>CEO: all tasks complete
    CEO->>Mission: mission_complete("PR opened: #142")
```

---

## 10. Prompt assembly — 4-layer pipeline

How the system prompt is built each turn.

<p align="center">
  <img src="../images/prompt_assembly.png" alt="Prompt assembly 4-layer pipeline" width="700">
</p>

---

## 11. Artifact storage and GC

How files, screenshots, and documents are stored and cleaned up.

<p align="center">
  <img src="../images/artifact.png" alt="Artifact storage and GC" width="750">
</p>
