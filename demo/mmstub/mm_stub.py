#!/usr/bin/env python3
"""Minimal Mattermost stub for the local demo (see demo/README.md).

Speaks just enough REST v4 for the delivery worker: POST /api/v4/posts
(create) and PUT /api/v4/posts/{id} (root refresh). Every message is
printed to stdout verbatim so you can watch the board update live.

Messages must never be re-encoded through json.dumps in the log path:
ascii-escaping buries the emoji the board renders with.
"""
import itertools
import json
import re
import sys
from http.server import BaseHTTPRequestHandler, HTTPServer

PORT = int(sys.argv[1]) if len(sys.argv) > 1 else 9999
counter = itertools.count(1)


class Handler(BaseHTTPRequestHandler):
    def _reply(self, obj):
        body = json.dumps(obj).encode()
        self.send_response(200)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):  # noqa: N802 - stdlib naming
        post = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        pid = f"post-{next(counter)}"
        kind = "reply " if post.get("root_id") else "ROOT  "
        print(f"\n=== {kind} -> {pid}"
              + (f" (thread {post['root_id']})" if post.get("root_id") else ""),
              flush=True)
        print(post.get("message", ""), flush=True)
        self._reply({"id": pid, "create_at": 1})

    def do_PUT(self):  # noqa: N802
        pid = re.sub(r".*/", "", self.path)
        post = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        print(f"\n=== ROOT REFRESH ({pid})", flush=True)
        print(post.get("message", ""), flush=True)
        self._reply({"id": pid})

    def log_message(self, *args):  # silence request logging; posts are the log
        pass


print(f"Mattermost stub listening on 127.0.0.1:{PORT}", flush=True)
HTTPServer(("127.0.0.1", PORT), Handler).serve_forever()
