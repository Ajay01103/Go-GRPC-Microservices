# TTS Service (Chatterbox on Modal)

This service runs multilingual text-to-speech using `chatterbox-tts` on Modal GPU workers.

Source file: `services/tts/chatterbox_tts.py`

## What This Service Does

- Exposes a FastAPI endpoint: `POST /generate`
- Requires API key auth via `x-api-key`
- Loads a reference voice WAV from S3-compatible object storage using `voice_key`
- Generates WAV audio with Chatterbox on `a10g` GPU

## Prerequisites

- Modal account and CLI installed
- Python 3.10+
- Access to an S3-compatible bucket containing voice files (for example `voices/system/default.wav`)
- Hugging Face token with permission to fetch model assets

## 1. Local Setup

From repository root:

```bash
cd services/tts
python -m venv .venv
# Linux/macOS
source .venv/bin/activate
# Windows PowerShell
# .venv\Scripts\Activate.ps1

pip install --upgrade pip
pip install modal
```

Authenticate Modal CLI:

```bash
modal setup
```

## 2. Required Modal Secrets

The deployed Modal app requires a secret named:

- `chatterbox-multi-tts`

Required keys (must exist in that secret):

- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`
- `AWS_REGION`
- `S3_BUCKET_NAME`
- `S3_ENDPOINT_URL`
- `CHATTERBOX_API_KEY`
- `HF_TOKEN`

### What Each Secret Is Used For

- `AWS_ACCESS_KEY_ID`: Access key for object storage read access
- `AWS_SECRET_ACCESS_KEY`: Secret for object storage read access
- `AWS_REGION`: Region used by the S3 client
- `S3_BUCKET_NAME`: Bucket containing voice files
- `S3_ENDPOINT_URL`: Endpoint for AWS S3 or S3-compatible provider
- `CHATTERBOX_API_KEY`: Shared key checked against incoming `x-api-key`
- `HF_TOKEN`: Token used by model dependencies during startup/download

### Create Secret in Modal

```bash
modal secret create chatterbox-multi-tts \
	AWS_ACCESS_KEY_ID="<your-access-key>" \
	AWS_SECRET_ACCESS_KEY="<your-secret-key>" \
	AWS_REGION="<your-region>" \
	S3_BUCKET_NAME="<your-bucket>" \
	S3_ENDPOINT_URL="<https://your-s3-endpoint>" \
	CHATTERBOX_API_KEY="<your-api-key>" \
	HF_TOKEN="<your-hf-token>"
```

## 3. Deploy to Modal

From repository root:

```bash
modal deploy services/tts/chatterbox_tts.py
```

Modal prints the deployed HTTPS URL for the ASGI app (for example: `https://<workspace>--chatterbox-multi-tts-chatterbox-serve.modal.run`).

## 4. Run a Remote Test Job (Optional)

This triggers `@app.local_entrypoint()` and writes a WAV file locally:

```bash
modal run services/tts/chatterbox_tts.py \
	--prompt "Hello from Chatterbox [chuckle]." \
	--voice-key "voices/system/default.wav" \
	--output-path "/tmp/chatterbox-output.wav"
```

## 5. Call the Deployed HTTP Endpoint

```bash
curl -X POST "https://<your-modal-endpoint>/generate" \
	-H "Content-Type: application/json" \
	-H "x-api-key: <your-api-key>" \
	-d '{
		"prompt": "Hello from Chatterbox [chuckle].",
		"voice_key": "voices/system/default.wav",
		"language_id": "en",
		"temperature": 0.8,
		"exaggeration": 0.5,
		"cfg_weight": 0.5
	}' \
	--output output.wav
```

## Request Contract

`POST /generate`

Required fields:

- `prompt` (string, 1..5000)
- `voice_key` (string, object key or URL path to a WAV in your bucket)

Optional fields:

- `language_id` (default: `en`)
- `temperature` (default: `0.8`)
- `exaggeration` (default: `0.5`)
- `cfg_weight` (default: `0.5`)

Backward-compatible but ignored fields:

- `top_p`
- `top_k`
- `repetition_penalty`
- `norm_loudness`

## Troubleshooting

- `403 Invalid API key`: `x-api-key` does not match `CHATTERBOX_API_KEY`
- `400 Voice not found`: `voice_key` does not exist in your bucket/path
- Startup/model errors: verify `HF_TOKEN` and that the Modal secret has all required keys
- S3 download failures: verify endpoint, bucket name, region, and access credentials

## Notes

- The service currently runs with `gpu="a10g"` and `@modal.concurrent(max_inputs=10)`.
- Voice keys can be provided as object keys or full URLs; the service normalizes them to bucket-relative keys.
