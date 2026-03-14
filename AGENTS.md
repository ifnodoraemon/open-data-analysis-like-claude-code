# Project Agent Guide

This repository follows an agentic runtime model.

## Core Principles

- The runtime provides tools, state, and guardrails. The model decides the path.
- Do not encode fixed workflows such as `analyze -> write -> finalize`.
- Prefer exposing facts through tools over injecting advice through prompts.
- Keep prompts short and operational. Put durable project guidance here, not in the runtime prompt.
- Keep tool descriptions factual: what the tool does, when to call it, and what side effects it has.
- Use thin guardrails only to block invalid final output or unsafe execution.

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
- Optional structures such as goals or report blocks are scaffolds, not mandatory thinking paths.
- Sub-agents are optional execution units, not separate personalities.

## Report Constraints

- Reports may be organized freely by the model.
- Finalize should only succeed when report state is structurally valid.
- Do not silently rewrite the report during finalize.

## Project References

- Agentic direction details: `docs/agentic-principles.md`
