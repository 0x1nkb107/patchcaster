# GitLab

PatchCaster takes merge request activity from GitLab 19.0 or later through signed webhooks, which arrive at the endpoint of a GitLab [connection](../operations.md#connections). One hook may cover a whole instance, a group or a single project. The [routes](../operations.md#teams-and-routes) alone decide which projects reach a channel.

## Webhook

Use a **Standard Webhooks**-capable hook (system, group, or project) with the
signing token enabled. PatchCaster verifies only the `webhook-signature` HMAC
(`v1,{base64}` over `{webhook-id}.{timestamp}.{body}`); the legacy
`X-Gitlab-Token` is never trusted.

* **URL**: the GitLab connection's `webhook_url`, `<public URL>/webhooks/<connection id>`, served by the public listener (`--addr`, `:8080` by default).
* **Signing token**: select **Generate signing token** in the hook form, and add the token to the connection at `/api/v1/connections/{id}/signing-tokens`, exactly as GitLab shows it, `whsec_` prefix included. GitLab shows it only once. A connection holds several signing tokens, one per hook or during a [rotation](../operations.md#connections).

## Triggers

Enable these GitLab trigger types — anything else is authenticated, counted
(`patchcaster_deliveries_discarded_total`), and dropped, so a missing trigger
degrades silently:

| Trigger | Effect |
| --- | --- |
| Merge request events | thread lifecycle, review verdicts, pushes, board state |
| Comment events | `💬` notices (merge-request notes only) |
| Pipeline events | `🟡/🟢/🔴` status segment; pass/fail notices |
| Push events | `N new commits` counts attributed by project + branch |
