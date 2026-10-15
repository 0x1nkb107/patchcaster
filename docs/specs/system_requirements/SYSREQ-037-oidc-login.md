---
id: "SYSREQ-037"
type: system_requirement
name: "OpenID Connect login"
description: "People log in only through the organization's OpenID Connect provider. PatchCaster keeps no passwords."
specification: "The service SHALL authenticate people only through the OpenID Connect provider an administrator configured, using the authorization code flow with PKCE, and SHALL keep no passwords of its own."
status: proposed
derives_from:
  - "SCEN-013"
---

# System Requirement: OpenID Connect login

## Requirement Specification

> The service SHALL authenticate people only through the OpenID Connect provider an administrator configured, using the authorization code flow with PKCE, and SHALL keep no passwords of its own.

## Rationale

The owner chose generic OpenID Connect for login (2026-10-15). An IT department already manages its people in an identity provider, and passwords of PatchCaster's own would be one more credential store to protect (ADR-033).

## Acceptance Criteria

- A person is identified by the issuer and subject of their ID token. Their first login creates their record with the role User.
- Sessions live on the server. Their cookie is `Secure`, `HttpOnly` and `SameSite=Lax`, and sessions expire and can be ended by logout.
- Every state-changing request of the web interface is protected against cross-site request forgery.
- An ID token whose issuer, audience, signature or expiry does not check out is refused.

## Verification Plan

- **Method**: Test.
- **Procedure**: Tests against a test identity provider cover the login, each refusal case, session expiry and a cross-site request that must be refused.
