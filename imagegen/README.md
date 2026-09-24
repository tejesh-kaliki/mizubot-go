# imagegen

Local image-generation host service for mizubot-go. Runs Qwen-Image-2.1
(MLX, 4-bit) via `mflux` directly on the Mac host — not in Docker, since
MLX needs Metal and containers on macOS can't see the GPU.

The bot container reaches this over `host.docker.internal:8081`
(`IMAGE_GEN_BASE_URL` in the bot's config).

## Run

```bash
cd imagegen
uv run server.py
```

First request after a fresh checkout will trigger an ~10.5GB model
download (`mlx-community/Qwen-Image-2.1-MLX-4bit`) via the mflux CLI;
subsequent requests reuse the cached weights but still reload them into
memory each time (on-demand, not resident — see server.py's docstring
for why).

Leave this running in a terminal (or set up a launchd agent) whenever
the bot's image-gen tool should be available.
