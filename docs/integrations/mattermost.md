# Mattermost

PatchCaster posts as a Mattermost **bot account** through the REST API v4:
it creates each thread's root post and replies with `POST /api/v4/posts`, and
refreshes the root's status board by editing the bot's own post with
`PUT /api/v4/posts/{id}`. Incoming webhooks are never
used: each is fixed to one channel and cannot thread replies reliably.

## Bot account

1. Create a bot account, for example `patchcaster`, and generate an access
   token for it. Bot account creation has to be enabled on the server;
   `mmctl` does both steps from the command line:

   ```sh
   mmctl bot create patchcaster --display-name PatchCaster
   mmctl token generate patchcaster "PatchCaster relay"
   ```

2. Create a Mattermost [connection](../operations.md#connections) with the token and the server's site URL. PatchCaster checks the token at once and shows the bot's account. The token only ever travels in the `Authorization` header.

3. Add the bot to the team and to every channel a [sink](../operations.md#teams-and-routes) names. PatchCaster checks the membership when it creates the sink:

   ```sh
   mmctl team users add <team> patchcaster
   mmctl channel users add <team>:<channel> patchcaster
   ```

## Channel ids

A sink wants the channel's **id**, not the name shown in channel URLs. With the bot's token, the API resolves a team and channel name to its id:

```sh
curl -s -H "Authorization: Bearer $BOT_TOKEN" \
  https://mattermost.example.com/api/v4/teams/name/<team>/channels/name/<channel> \
  | jq -r .id
```

## When posts fail

* **Missing membership or a bad token**: Mattermost refuses the post, and
  PatchCaster retries with backoff before parking the message
  (`patchcaster_messages_parked_total{cause="attempt_budget"}`); the refusal
  is kept in the outbox row's `last_error`, and the sink's
  [health](../operations.md#connection-and-sink-health) shows it. See
  [delivery guarantees](../operations.md#delivery-guarantees).
* **A deleted root post** counts as an intentional removal: work for that
  thread parks with `cause="thread_gone"`, and PatchCaster does not post the
  thread again.
* **A post edit time limit** (`ServiceSettings.PostEditTimeLimit`) also
  binds the bot: once a root post is older than the limit, its status board
  can no longer be refreshed. Keep the limit off on servers PatchCaster
  posts to.
