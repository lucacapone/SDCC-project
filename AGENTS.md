<!-- filename: AGENTS.md -->

# AGENTS — SDCC Gossip-based Distributed Data Aggregation Project

## 0) Mandatory Language Rule

**All responses and outputs must always be in Italian, regardless of the language used by the user.**
No exceptions are allowed.

---

# 1) Agent Roles and Authority Structure

This project follows a strict agent hierarchy.

## Codex — Primary Repository Execution Agent

**Codex is the primary repository-aware execution agent.**

Codex is responsible for:

* reading project files
* performing repository-grounded analysis
* implementing changes
* writing and modifying code
* creating tests
* validating system behavior
* updating documentation
* maintaining project consistency

Codex is responsible for both **design support and implementation activities inside the repository**.

All repository modifications must be executed by Codex.

---

## GPT — Support Agent

GPT is **not the primary execution agent**.

GPT may only be used for:

* high-level reasoning
* architectural discussions with the human
* web search or deep research
* generating structured prompts requested by Codex

GPT does **not perform repository modifications**.

---

## Human — Final Decision Authority

The human remains the **final authority** for:

* architectural decisions
* requirement interpretation
* conflict resolution
* approval of major changes.

When documentation does not clearly resolve a decision, Codex must escalate to the human.

---

# 2) Mandatory Read-Before-Write Rule

Before performing **any modification to the repository**, Codex **must read**:

1. The current project documentation
2. The architecture description
3. The project report draft (if present)
4. Existing configuration files
5. Testing documentation
6. The operational log
7. Any operational notes referenced by documentation

Codex must never modify files **without first understanding the current repository state**.

---

# 3) Source of Truth Protection Rule

Some documents may be marked as **`source_of_truth`**.

Codex **must not modify any document marked as `source_of_truth`** unless:

* the human explicitly requests that exact modification.

If such a request exists, Codex must:

* clearly document the modification
* update logs accordingly.

---

# 4) Mandatory Activity Logging

Every activity performed by Codex **must be logged**.

Logging requirements:

* Log file location:

```
docs/operational_log.md
```

Rules:

* Each log entry must **append to the file**.
* Existing entries must **never be overwritten**.
* Each entry must include:

  * date
  * time
  * description of the task
  * files modified
  * reasoning summary.

The goal is to allow **full reconstruction of the project's evolution over time**.

---

# 5) Documentation Consistency Rule

For **every modification**, Codex must ensure:

* documentation remains consistent with the implementation
* architecture diagrams match the actual system
* configuration documentation reflects real parameters.

If code and documentation diverge:

**Codex must resolve the inconsistency within the same change scope**, unless the human explicitly instructs otherwise.

---

# 6) Codex Operational Workflow

Codex must follow the operational workflow below.

---

## Step 1 — Repository Analysis

Codex must first:

* read project documentation
* read architecture descriptions
* read previous logs
* understand existing modules and components.

No changes may begin before this analysis.

---

## Step 2 — Task Understanding

Codex must determine:

* the goal of the requested change
* the scope of the change
* affected modules
* expected system behavior.

If the objective is unclear, escalation is required.

---

## Step 3 — Implementation

Codex implements changes in the repository.

Typical actions include:

* writing Go code
* implementing gossip protocol logic
* implementing aggregation algorithms
* adding configuration handling
* adding Docker configuration
* implementing tests.

---

## Step 4 — Validation

Codex must validate the implementation through:

* compilation checks
* test execution
* correctness verification.

Testing should confirm:

* node communication
* gossip convergence
* correctness of aggregation.

---

## Step 5 — Documentation Update

After implementing changes, Codex must update:

* architecture documentation
* configuration documentation
* testing documentation
* project report sections if needed.

---

## Step 6 — Operational Logging

After completing the work, Codex must append a log entry to:

```
docs/operational_log.md
```

The entry must include:

* date and time
* objective
* summary of work performed
* list of files modified.

---

# 7) Escalation Rule (Critical)

**This rule is mandatory and critical.**

If Codex encounters any situation where:

* documentation is unclear
* the correct design choice is ambiguous
* the requested change conflicts with existing documentation
* the decision requires architectural judgment not specified in the repository

Codex **must stop immediately**.

Codex must then:

1. describe the problem
2. list possible options
3. ask the human to clarify or decide.

Codex **must not proceed until the human responds**.

---

# 8) Handling Documentation / Code Divergence

If Codex discovers divergence between:

* code and documentation
* configuration and implementation
* architecture description and actual behavior

Codex must:

1. identify the inconsistency
2. resolve it during the same change
3. update documentation and logs.

Only if the human explicitly instructs otherwise may Codex leave inconsistencies unresolved.

---

# 9) Repository Change Requirements

All modifications performed by Codex must:

* preserve system correctness
* preserve distributed system properties
* respect project constraints:

  * Go implementation
  * gossip-based communication
  * decentralized aggregation
  * configurable parameters
  * Docker-based deployment
  * AWS-compatible deployment.

---

# 10) Mandatory Code Quality Constraints

For every code change, Codex **must** ensure that all Codex-written code:

* is fully commented;
* is code-smell-free to the extent reasonably achievable;
* follows clean-code principles and good coding practices.

Codex **must not** intentionally introduce avoidable code smells or violations of clean-code principles in Codex-written code.

---

# 11) Codex ↔ GPT Handoff Protocol

When Codex needs a **capability that only GPT can provide** (for example web research):

Codex must follow this protocol.

1. Codex writes a **clear prompt** addressed to GPT.
2. Codex instructs GPT to return the output **in Markdown format**.
3. Codex provides the prompt to the human.
4. The human executes the prompt using GPT.
5. GPT returns Markdown output.
6. The human adds the Markdown output to the repository.
7. Codex resumes work using that repository-added content.

No other GPT policy or usage is allowed inside this document.
