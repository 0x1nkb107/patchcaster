# GitLab

PatchCaster takes merge request activity from GitLab 19.0 or later through a
signed webhook. One hook may cover a whole instance, a group or a single
project; the [routing table](../configuration.md#routing-table) alone decides
which projects reach a channel.

## Webhook

Use a **Standard Webhooks**-capable hook (system, group, or project) with the
signing token enabled. PatchCaster verifies only the `webhook-signature` HMAC
(`v1,{base64}` over `{webhook-id}.{timestamp}.{body}`); the legacy
`X-Gitlab-Token` is never trusted.

* **URL**: `https://<patchcaster-host>/webhook` — the endpoint is
  `POST /webhook` on the public listener (`--addr`, `:8080` by default).
* **Signing token**: select **Generate signing token** in the hook form, and give the token to PatchCaster as a `--signing-secret` value exactly as GitLab shows it, `whsec_` prefix included. GitLab shows it only once. Several may be active while the token [rotates](../configuration.md#patchcaster-serve).

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
