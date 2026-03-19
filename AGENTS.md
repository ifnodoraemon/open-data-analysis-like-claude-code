# Project Agent Guide

This repository follows an agentic runtime model.

## Core Principles

- The runtime provides tools, state, and guardrails. The model decides the path.
- Do not encode fixed workflows such as `analyze -> write -> finalize`.
- Prefer exposing facts through tools over injecting advice through prompts.
- Keep prompts short and operational. Put durable project guidance here, not in the runtime prompt.
- Keep tool descriptions factual and contract-oriented: what the tool does, when it applies, when it does not, what state it reads/writes, and what it returns.
- Use thin guardrails only to block invalid final output or unsafe execution.
- When a requested analysis depends on ambiguous metric definitions, join keys, time grains, units, or field mappings, do not silently lock in one interpretation. Let the agent inspect facts, then ask the user or make an explicit assumption only when the user has allowed that tradeoff.
- Do not return `next_action`-style advice from tools.
- Do not inject hidden workflow hints such as “first call tool X” into handler-assembled user messages.
- Keep UI summaries separate from fact payloads. Prefer `ui_summary` for display/logging fields; do not introduce new `summary_text` writes.

## Prompt Style

- Prefer short runtime prompts over long behavioral manifestos.
- Task framing should be concrete:
  - Goal
  - Context
  - Constraints
  - Done when
- Avoid repeating the same philosophy in the system prompt and in every tool description.

## Tool Design

- Observation tools return facts, not advice.
- Action tools should describe state changes clearly.
- Tool descriptions may be detailed, but they must not prescribe the model's next step.
- Optional structures such as goals or report blocks are scaffolds, not mandatory thinking paths.
- Sub-agents are optional execution units, not separate personalities.
- Prefer pull-based state access through `state_*` tools over automatic prompt injection of large runtime state.
- If a tool needs a human-readable display summary, return it in `ui_summary` and keep model-relevant facts in separate structured fields.

## Report Constraints

- Reports may be organized freely by the model.
- Finalize should only succeed when report state is structurally valid.
- Do not silently rewrite the report during finalize.

## Project References

- Agentic direction details: `docs/agentic-principles.md`
