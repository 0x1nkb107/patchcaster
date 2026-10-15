---
id: "SCEN-021"
type: scenario
name: "An operator customizes a template with a preview"
description: "An operator rewrites a route's root template in Markdown, previews it for each sink platform, and saves it after validation."
status: proposed
refines:
  - "UC-005"
---

# Scenario: An operator customizes a template with a preview

## Overview

Teams want their own wording. Template changes must neither break delivery nor let forge content inject formatting.

## Initial State

- A route uses the built-in templates.

## Trigger

The operator edits the route's root template.

## Step-by-Step Flow

1. The editor shows the documented data model of the template's version.
2. The preview renders the draft against sample events, once for each sink platform: Markdown for Mattermost, HTML for Matrix.
3. Saving parses the template and renders every sample event. Only a template that passes both is saved.
4. Later roots use the new template. Existing roots take it on their next board refresh.

## Expected Outcome

- The route's messages read the way the operator wrote them.
- A merge request title such as `@channel [click](https://example.com)` still appears literally, whatever the template does with it.

## Exceptions & Edge Cases

- A template that does not parse, or fails on a sample event, is refused, with the line and the cause.
- When a release brings a new data model version, saved templates keep rendering against the version they were written for.
