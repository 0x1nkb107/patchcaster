# Message templates

Every message that PatchCaster posts comes from a template: the root message that starts the thread of a merge request, and each notice in that thread. Built-in templates ship with PatchCaster, and a route may replace any of them with its own. This page describes how templates are written, what they can read, the checks a template has to pass, and how a route uses them.

## Messages

| Kind | Posted when |
| --- | --- |
| `root` | a merge request opens; the root is then refreshed whenever its status changes |
| `comment` | someone comments |
| `pipeline_passed` | a pipeline passes |
| `pipeline_failed` | a pipeline fails |
| `reviewed` | a reviewer reviews |
| `changes_requested` | a reviewer asks for changes |
| `approved` | a reviewer approves |
| `pushed` | someone pushes commits |
| `merged` | the merge request is merged |
| `closed` | the merge request is closed |

## Writing a template

A template is a Go [text/template](https://pkg.go.dev/text/template) that writes Markdown: CommonMark with the GitHub Flavored Markdown extensions for tables, strikethrough and autolinks. PatchCaster renders that Markdown into each chat platform's own format. Mattermost shows every construct, apart from two that degrade: an image becomes a link to the image, and raw HTML shows as text. Links lead only to `http`, `https` and `mailto` addresses; any other link shows as its text.

Templates can print fields (`{{.MergeRequest.Title}}`), choose with `{{if}}`, `{{else}}` and `{{with}}`, set variables, and call the [functions](#functions) below. `range`, `define`, `block` and `template` are not available, and neither are the other functions of text/template.

## Data model version 1

| Field | Type | Meaning |
| --- | --- | --- |
| `.MergeRequest.Number` | number | the merge request's number in its project |
| `.MergeRequest.Title` | forge text | the title |
| `.MergeRequest.Author` | forge text | the username of the author |
| `.MergeRequest.Project` | forge text | the path of the project |
| `.MergeRequest.SourceBranch` | forge text | the branch that the merge request merges |
| `.MergeRequest.TargetBranch` | forge text | the branch it merges into |
| `.MergeRequest.URL` | address | the merge request's web address; empty unless it is an `http` or `https` address |
| `.MergeRequest.Open` | true or false | whether the merge request is open |
| `.Status.Pipeline` | word | the latest pipeline: `running`, `passed`, `failed`, `waiting`, `canceled`, `skipped`, or `unknown` for a state PatchCaster does not know; empty before the first pipeline |
| `.Status.ReviewersDone` | number | the reviewers who reviewed or approved |
| `.Status.ReviewersRequired` | number | the reviewers the merge request needs |
| `.Status.Comments` | number | the comments |
| `.Status.Edits` | number | the pushes and description changes |
| `.Status.Line` | forge text | the status line of the built-in root, empty when there is no status |
| `.Event.Kind` | word | the kind of notice, as in [messages](#messages); empty for the root |
| `.Event.Actor` | forge text | the username of whoever acted; empty for the root |
| `.Event.Commits` | number | the commits of a push, or 0 when the forge gave no count |

Every message reads `.MergeRequest` and `.Status` as they stand when PatchCaster posts it. Only notices have an `.Event`.

### Forge text

Anyone who can open a merge request chooses its title, so forge text reaches templates escaped. Printed, it shows exactly as written, and it can add no formatting, link or mention, whatever the template does around it. Use `code` to show forge text as code: backticks around it would show the escaping.

The address is ready to be a link: write `<{{.MergeRequest.URL}}>` or `[!{{.MergeRequest.Number}}]({{.MergeRequest.URL}})`. Forge text that holds an address does not become a link.

## Functions

| Function | Writes |
| --- | --- |
| `mention "here"` | a mention of the people in the channel who are online |
| `mention "channel"` | a mention of everyone in the channel |
| `code` forge text | the forge text as a code span, such as `{{code .MergeRequest.SourceBranch}}` |
| `plural` number singular plural | the number with the right noun, such as `{{plural .Status.Comments "comment" "comments"}}` for `1 comment` or `3 comments` |

Conditions can compare with `eq`, `ne`, `lt`, `le`, `gt` and `ge`, and combine with `and`, `or` and `not`. Only the functions above write mentions: `@here` written in a template shows as text.

## Checks

PatchCaster checks a template before it saves it. The template has to parse and to use only the fields and functions on this page. It then has to render a set of sample merge requests: each pipeline state, open and closed, without an address, and for notices with and without a commit count. A refused template names the line and the cause.

The samples include the longest values a forge sends: titles, usernames and branch names of 255 characters, a project path of 1,024 characters and an address of 2,048 characters, each made of the character whose escaping grows most. A template whose message would then hold more than 16,383 characters for Mattermost is refused. Every Mattermost server in use accepts messages of that length.

## Versions

A template records the version of the data model it was written for. Later releases may add a new version, but they keep rendering every template in the version it was written for.

## Templates of a route

A route's Routing Managers, or an Instance Admin, give the route a template of its own for one kind of message at a time, through the management API:

- `PUT /api/v1/routes/{id}/templates/{kind}` with `{"version": 1, "text": "..."}` stores the template. `If-Match` holds the route's ETag, and the answer carries the route's new one. A template that fails its [checks](#checks) answers `422`, naming the line and the cause in the error of the field `text`.
- `GET` on the same path reads the template, and `DELETE` returns the kind to its built-in template.
- The route's `custom_templates` lists the kinds it has templates of its own for.
- `POST /api/v1/templates/preview` with `{"kind": "root", "version": 1, "text": "..."}` shows a draft as it would reach Mattermost for each sample, without storing it.

A change applies to the next message, and a thread's root shows it at its next refresh. The audit log shows each template before and after its change.

When several routes reach one sink, the route with the narrowest source writes the sink's messages: a repository before a group, a deeper group before a shallower one, and a group before the whole instance. Among equally narrow routes, the oldest one writes. A thread keeps the route that wrote its root for as long as that route exists, so its messages keep one look.

A template that passed its checks can still fail on a merge request that no sample resembled. PatchCaster then writes that message with the built-in template, logs a warning that names the route and the kind, and counts it in `patchcaster_template_fallbacks_total`.

## Built-in templates

The built-in templates read data model version 1. The root:

```
> {{mention "here"}} :bell: \
> **!{{.MergeRequest.Number}} {{.MergeRequest.Title}}** \
> by {{.MergeRequest.Author}} • {{.MergeRequest.Project}} • {{code .MergeRequest.SourceBranch}} ➜ {{code .MergeRequest.TargetBranch}} \
> <{{.MergeRequest.URL}}>{{with .Status.Line}} \
> {{.}}{{end}}
```

The notices:

| Kind | Template |
| --- | --- |
| `comment` | `💬 {{.Event.Actor}} commented` |
| `pipeline_passed` | `🟢 Pipeline passed` |
| `pipeline_failed` | `🔴 Pipeline failed` |
| `reviewed` | `👀 {{.Event.Actor}} reviewed the MR` |
| `changes_requested` | `🚨 {{.Event.Actor}} asks for changes!` |
| `approved` | `✅ Review was approved by {{.Event.Actor}}!` |
| `pushed` | `📝 {{.Event.Actor}} pushed {{with .Event.Commits}}{{.}} {{end}}new commits` |
| `merged` | `🎉 {{.Event.Actor}} merged !{{.MergeRequest.Number}}` |
| `closed` | `🔒 {{.Event.Actor}} closed !{{.MergeRequest.Number}}` |
