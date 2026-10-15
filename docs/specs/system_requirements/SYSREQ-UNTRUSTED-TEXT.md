---
id: "SYSREQ-051"
type: system_requirement
name: "Forge text is escaped before templates run"
description: "Every value that comes from a forge reaches templates already escaped, so forge content cannot add formatting, links or mentions."
specification: "The service SHALL escape every value that originates from a forge for Markdown before a template receives it, so that no forge content can add formatting, links or mentions to a message."
status: proposed
derives_from:
  - "SCEN-021"
depends_on:
  - "SYSREQ-003"
---

# System Requirement: Forge text is escaped before templates run

## Requirement Specification

> The service SHALL escape every value that originates from a forge for Markdown before a template receives it, so that no forge content can add formatting, links or mentions to a message.

## Rationale

Anyone who may open a merge request can choose its title. Once operators write templates, escaping cannot be left to each template author. It has to happen before the data reaches any template.

## Acceptance Criteria

- Titles, branch names, user names, labels and comment excerpts are escaped. URLs the forge supplies are passed as link targets only.
- A title such as `@channel [click](https://example.com)` appears literally on every sink platform.
- Mentions arise only from the template's mention function and linked identities (SYSREQ-054). Room-wide mentions such as `@room`, `@channel` or `@here` never arise from forge data.

## Verification Plan

- **Method**: Test.
- **Procedure**: Fuzz tests feed hostile titles and comments through every built-in template and every platform renderer, and assert the rendered output holds no formatting, link or mention from the input.
