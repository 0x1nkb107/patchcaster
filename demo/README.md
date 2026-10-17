# Local demo

Watch the live status board without GitLab, Mattermost, or the internet:
a stub Mattermost prints every post, and an event simulator delivers signed
GitLab webhooks. All defaults are mutual — copy the commands, they work.

## Terminals

1. **Database** (once): `just devdb`
2. **Stub Mattermost**: `python3 demo/mmstub/mm_stub.py` (port arg optional, default 9999; every post prints here)
3. **Service**:

   ```sh
   PATCHCASTER_MATTERMOST_URL=http://127.0.0.1:9999 \
   PATCHCASTER_MATTERMOST_TOKEN=stub \
   PATCHCASTER_SIGNING_SECRETS="$(./demo/evmock/evmock.sh token)" \
   just run
   ```

4. **Events**: `./demo/evmock/evmock.sh <kind>` — try them in order:

   ```sh
   ./demo/evmock/evmock.sh open             # root post appears in terminal 2
   ./demo/evmock/evmock.sh comment          # 💬 reply in the thread + root refresh
   ./demo/evmock/evmock.sh pipeline-pass    # 🟢
   ./demo/evmock/evmock.sh push             # 📝 N new commits (two webhooks, like GitLab)
   ./demo/evmock/evmock.sh reviewed         # 👀
   ./demo/evmock/evmock.sh approval         # ✅
   ./demo/evmock/evmock.sh changes          # 🚨
   ./demo/evmock/evmock.sh pipeline-fail    # 🔴
   ./demo/evmock/evmock.sh merge            # 🎉 and the thread closes
   ```

Each invocation signs with `SIGNING_SECRET` (Standard Webhooks) and stamps a
fresh event timestamp, so the 4 h freshness cap (SYSREQ-013) stays satisfied;
`evmock.sh` with no argument lists kinds and env knobs. The project path
defaults to `group/application`, a route from `contrib/routing.example.yaml`.

Re-running the sequence: either finish the previous thread with `merge`/
`close`, or pass a fresh `MR_ID`/`MR_IID` (and optionally `SOURCE_BRANCH`).
An open thread per project+branch is what lets push hooks attribute their
commit count; two concurrent MRs on one branch is deliberate ambiguity and
falls back to the count-less 📝 (SYSREQ-030).

To watch the durable machinery instead of posts: run without the stub (any
unreachable Mattermost URL) and observe `curl localhost:9090/metrics` plus
the `outbox` table — deliveries park and back off, then drain when the stub
comes back. Ctrl-C the service to see the documented drain (SYSREQ-023).
