# AI Filter

Admin-configurable content filter. Bleeps chosen words and removes scene types (sexual, graphic violence, drug use, self-harm, etc). Built-in `r-to-pg13` and `pg13-to-pg` profiles; admins can add custom word lists and categories.

Produces a filter **plan** (filtered SRT + EDL cuts). Playback/transcode peers apply the plan — this module does not rewrite media files itself.

## Ports

| Service | Default |
|---------|---------|
| gRPC | `127.0.0.1:9768` |
| HTTP | `127.0.0.1:9769` |

## HTTP

| Method | Path |
|--------|------|
| GET | `/v1/profiles` |
| POST | `/v1/profiles` |
| POST | `/v1/apply` |
