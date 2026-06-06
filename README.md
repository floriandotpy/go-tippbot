# go-tippbot

AI agent that predicts football match scores and submits them to a [GoTipp](https://github.com/flo/gotipp) instance. Uses an LLM (any OpenAI-compatible endpoint) via [Genkit Go](https://genkit.dev/docs/go/get-started/).

Run it daily — it skips matches that already have a tipp.

## Setup

Requires Go 1.24+.

```bash
cp .env.example .env
```

Fill in `.env`:

| Variable | Description |
|----------|-------------|
| `LLM_API_KEY` | API key for your model provider |
| `LLM_BASE_URL` | OpenAI-compatible base URL (e.g. `https://api.openai.com/v1`) |
| `LLM_MODEL` | Model name (e.g. `gpt-4o`, `claude-sonnet-4-20250514`) |
| `GOTIPP_API_TOKEN` | Bearer token from GoTipp settings |
| `GOTIPP_BASE_URL` | GoTipp server URL (default: `http://localhost:8080`) |

## Run

```bash
go run .
```

Or build a binary:

```bash
go build -o tippbot .
./tippbot
```

## Scheduling

Cron example (daily at 8am):

```
0 8 * * * cd /path/to/tippbot && ./tippbot >> /tmp/tippbot.log 2>&1
```
