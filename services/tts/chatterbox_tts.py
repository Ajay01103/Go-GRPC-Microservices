"""Chatterbox TTS API - Text-to-speech with voice cloning on Modal."""

import modal

# Secret setup in Modal dashboard:
# - chatterbox-multi-tts: AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION,
#   S3_BUCKET_NAME, S3_ENDPOINT_URL, CHATTERBOX_API_KEY, HF_TOKEN

# Use this to test locally:
# modal run tts/chatterbox_tts.py \
#   --prompt "Hello from Chatterbox [chuckle]." \
#   --voice-key "voices/system/<voice-id>"

# Use this to test CURL:
# curl -X POST "https://<your-modal-endpoint>/generate" \
#   -H "Content-Type: application/json" \
#   -H "X-Api-Key: <your-api-key>" \
#   -d '{"prompt": "Hello from Chatterbox [chuckle].", "voice_key": "voices/system/<voice-id>"}' \
#   --output output.wav

image = modal.Image.debian_slim(python_version="3.10").uv_pip_install(
    "chatterbox-tts==0.1.7",
    "fastapi[standard]==0.115.4",
    "peft==0.13.2",
    "boto3==1.37.0",
)
app = modal.App("chatterbox-multi-tts", image=image)

with image.imports():
    import io
    import os
    import tempfile
    import urllib.parse
    from pathlib import Path

    import boto3
    import torchaudio as ta
    from botocore.config import Config
    from botocore.exceptions import ClientError
    from chatterbox.mtl_tts import ChatterboxMultilingualTTS
    from fastapi import Depends, FastAPI, HTTPException, Security
    from fastapi.middleware.cors import CORSMiddleware
    from fastapi.responses import StreamingResponse
    from fastapi.security import APIKeyHeader
    from pydantic import BaseModel, Field

    api_key_scheme = APIKeyHeader(
        name="x-api-key",
        scheme_name="ApiKeyAuth",
        auto_error=False,
    )

    def require_env(key: str) -> str:
        value = os.environ.get(key, "").strip()
        if not value:
            raise RuntimeError(f"Missing required environment variable: {key}")
        return value

    def verify_api_key(x_api_key: str | None = Security(api_key_scheme)):
        expected = os.environ.get("CHATTERBOX_API_KEY", "")
        if not expected or x_api_key != expected:
            raise HTTPException(status_code=403, detail="Invalid API key")
        return x_api_key

    class TTSRequest(BaseModel):
        """Request model for text-to-speech generation."""

        prompt: str = Field(..., min_length=1, max_length=5000)
        voice_key: str = Field(..., min_length=1, max_length=300)
        language_id: str = Field(default="en", min_length=2, max_length=10)
        # Defaults tuned for steadier pacing on cloned voices.
        temperature: float = Field(default=0.6, ge=0.0, le=2.0)
        exaggeration: float = Field(default=0.35, ge=0.25, le=2.0)
        cfg_weight: float = Field(default=0.35, ge=0.2, le=1.0)
        # Backward-compatible fields kept temporarily and ignored by model.generate.
        top_p: float | None = Field(default=None)
        top_k: int | None = Field(default=None)
        repetition_penalty: float | None = Field(default=None)
        norm_loudness: bool | None = Field(default=None)


@app.cls(
    gpu="a10g",
    scaledown_window=60 * 5,
    secrets=[
        modal.Secret.from_name(
            "chatterbox-multi-tts",
            required_keys=[
                "AWS_ACCESS_KEY_ID",
                "AWS_SECRET_ACCESS_KEY",
                "AWS_REGION",
                "S3_BUCKET_NAME",
                "S3_ENDPOINT_URL",
                "CHATTERBOX_API_KEY",
                "HF_TOKEN",
            ],
        ),
    ],
)
@modal.concurrent(max_inputs=10)
class Chatterbox:
    @modal.enter()
    def load_model(self):
        self.model = ChatterboxMultilingualTTS.from_pretrained(device="cuda")
        self.bucket_name = require_env("S3_BUCKET_NAME")
        self.s3 = boto3.client(
            "s3",
            endpoint_url=require_env("S3_ENDPOINT_URL"),
            aws_access_key_id=require_env("AWS_ACCESS_KEY_ID"),
            aws_secret_access_key=require_env("AWS_SECRET_ACCESS_KEY"),
            region_name=require_env("AWS_REGION"),
            config=Config(s3={"addressing_style": "path"}),
        )

    def _normalize_voice_key(self, voice_key: str) -> str:
        key = (voice_key or "").strip()
        if not key:
            return ""

        parsed = urllib.parse.urlparse(key)
        if parsed.scheme and parsed.netloc:
            key = parsed.path

        key = key.lstrip("/")

        bucket_prefix = f"{self.bucket_name}/"
        if key.startswith(bucket_prefix):
            key = key[len(bucket_prefix) :]

        voices_idx = key.find("voices/")
        if voices_idx > 0:
            key = key[voices_idx:]

        return key

    @modal.asgi_app()
    def serve(self):
        web_app = FastAPI(
            title="Chatterbox TTS API",
            description="Text-to-speech with voice cloning",
            docs_url="/docs",
            dependencies=[Depends(verify_api_key)],
        )
        web_app.add_middleware(
            CORSMiddleware,
            allow_origins=["*"],
            allow_credentials=True,
            allow_methods=["*"],
            allow_headers=["*"],
        )

        @web_app.post("/generate", responses={200: {"content": {"audio/wav": {}}}})
        def generate_speech(request: TTSRequest):
            try:
                audio_bytes = self.generate_from_voice_key.local(
                    request.prompt,
                    request.voice_key,
                    request.language_id,
                    request.temperature,
                    request.exaggeration,
                    request.cfg_weight,
                )
                return StreamingResponse(io.BytesIO(audio_bytes), media_type="audio/wav")
            except ValueError as e:
                raise HTTPException(status_code=400, detail=str(e)) from e
            except Exception as e:
                raise HTTPException(
                    status_code=500,
                    detail=f"Failed to generate audio: {e}",
                ) from e

        return web_app

    @modal.method()
    def generate_from_voice_key(
        self,
        prompt: str,
        voice_key: str,
        language_id: str = "en",
        temperature: float = 0.6,
        exaggeration: float = 0.35,
        cfg_weight: float = 0.35,
    ):
        tmp_path: str | None = None
        normalized_key = self._normalize_voice_key(voice_key)

        if not normalized_key:
            raise ValueError("voice_key is required")

        try:
            with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as tmp:
                self.s3.download_fileobj(self.bucket_name, normalized_key, tmp)
                tmp_path = tmp.name
        except ClientError as e:
            code = e.response.get("Error", {}).get("Code", "")
            if code in ("404", "NoSuchKey", "NotFound"):
                raise ValueError(f"Voice not found at '{normalized_key}'") from e
            raise RuntimeError(f"S3 download failed: {e}") from e

        try:
            wav = self.model.generate(
                prompt,
                language_id=language_id,
                audio_prompt_path=tmp_path,
                temperature=temperature,
                exaggeration=exaggeration,
                cfg_weight=cfg_weight,
            )

            buffer = io.BytesIO()
            ta.save(buffer, wav, self.model.sr, format="wav")
            buffer.seek(0)
            return buffer.read()
        finally:
            if tmp_path:
                Path(tmp_path).unlink(missing_ok=True)


@app.local_entrypoint()
def test(
    prompt: str = "Chatterbox running on Modal [chuckle].",
    voice_key: str = "voices/system/default.wav",
    language_id: str = "en",
    output_path: str = "/tmp/chatterbox-tts/output.wav",
    temperature: float = 0.6,
    exaggeration: float = 0.35,
    cfg_weight: float = 0.35,
):
    import pathlib

    chatterbox = Chatterbox()
    audio_bytes = chatterbox.generate_from_voice_key.remote(
        prompt=prompt,
        voice_key=voice_key,
        language_id=language_id,
        temperature=temperature,
        exaggeration=exaggeration,
        cfg_weight=cfg_weight,
    )

    output_file = pathlib.Path(output_path)
    output_file.parent.mkdir(parents=True, exist_ok=True)
    output_file.write_bytes(audio_bytes)
    print(f"Audio saved to {output_file}")