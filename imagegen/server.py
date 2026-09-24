"""Local image-generation host service for mizubot-go.

Runs on the Mac host (not in Docker — MLX needs Metal, which containers
can't see). The bot container reaches this over host.docker.internal.

Contract (must match internal/imagegen/client.go):
    POST /generate
    Content-Type: application/json
    Body: {"prompt": "<string>"}

    200 OK, Content-Type: image/png -> raw PNG bytes in body
    non-200 -> plain-text error message in body

Each request shells out to the mflux CLI, which loads the ~10.5GB
quantized model fresh, generates, and exits. That's deliberate: it keeps
the model out of memory between requests on a 16GB machine, at the cost
of a per-request load penalty. See the imagegen/README for the tradeoff.
"""

import json
import os
import random
import subprocess
import sys
import tempfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

HOST = "0.0.0.0"
PORT = int(os.environ.get("IMAGEGEN_PORT", "8081"))

VENV_BIN = os.path.join(os.path.dirname(os.path.abspath(__file__)), ".venv", "bin")
MFLUX_BIN = os.path.join(VENV_BIN, "mflux-generate-qwen-2.1")
MODEL = "mlx-community/Qwen-Image-2.1-MLX-4bit"

# Keep comfortably under the Go side's IMAGE_GEN_TIMEOUT (120s default) and
# the bot's overall 3-minute message-handling budget.
GENERATE_TIMEOUT_S = 100

WIDTH = 512
HEIGHT = 512
STEPS = 20


class Handler(BaseHTTPRequestHandler):
    def log_message(self, fmt, *args):
        sys.stderr.write("%s - %s\n" % (self.address_string(), fmt % args))

    def _send_text(self, status, text):
        body = text.encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        if self.path != "/generate":
            self._send_text(404, "not found")
            return

        length = int(self.headers.get("Content-Length", "0") or "0")
        raw = self.rfile.read(length) if length else b""
        try:
            payload = json.loads(raw or b"{}")
        except json.JSONDecodeError:
            self._send_text(400, "invalid json body")
            return

        prompt = (payload.get("prompt") or "").strip()
        if not prompt:
            self._send_text(400, "missing 'prompt'")
            return

        with tempfile.TemporaryDirectory() as tmpdir:
            out_path = os.path.join(tmpdir, "out.png")
            seed = random.randint(0, 1_000_000_000)
            cmd = [
                MFLUX_BIN,
                "--model", MODEL,
                "--low-ram",
                "--prompt", prompt,
                "--width", str(WIDTH),
                "--height", str(HEIGHT),
                "--steps", str(STEPS),
                "--seed", str(seed),
                "--output", out_path,
            ]
            try:
                proc = subprocess.run(
                    cmd,
                    capture_output=True,
                    text=True,
                    timeout=GENERATE_TIMEOUT_S,
                )
            except subprocess.TimeoutExpired:
                self._send_text(504, "image generation timed out")
                return

            if proc.returncode != 0 or not os.path.exists(out_path):
                err = (proc.stderr or proc.stdout or "unknown error").strip()
                sys.stderr.write("generation failed: %s\n" % err)
                self._send_text(500, err[:2000] or "generation failed")
                return

            with open(out_path, "rb") as f:
                image_bytes = f.read()

        self.send_response(200)
        self.send_header("Content-Type", "image/png")
        self.send_header("Content-Length", str(len(image_bytes)))
        self.end_headers()
        self.wfile.write(image_bytes)


def main():
    server = ThreadingHTTPServer((HOST, PORT), Handler)
    print(f"imagegen server listening on {HOST}:{PORT}, model={MODEL}")
    try:
        server.serve_forever()
    except KeyboardInterrupt:
        pass


if __name__ == "__main__":
    main()
