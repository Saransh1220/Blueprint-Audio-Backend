# Audio Metadata Analysis — Implementation Plan

Status: planning only  
Owner: Saransh  
Last updated: 2026-07-31  
Progress tracker: `docs/audio-metadata-analysis-progress.md`

## 1. Objective

Build a separate Python audio-analysis service that analyzes a producer's verified preview MP3 while the upload form is still open and suggests:

- BPM
- Musical key and scale
- Genre
- Moods
- Instruments
- Up to three searchable tags
- Basic technical metadata such as duration, sample rate, and channel count

The producer remains in control. Suggestions must never silently overwrite fields the producer has already edited, and analysis failure must never prevent manual publishing.

## 2. Existing foundation

The following capabilities already exist and should be reused:

- Angular creates an upload draft when the first file is selected.
- Angular uploads files directly to private R2 using presigned PUT URLs.
- The Go API verifies each uploaded object using size, content type, and ETag.
- PostgreSQL is already used as a durable background-job queue.
- The existing Go worker uses job claims, worker IDs, heartbeats, leases, stale-job recovery, and fenced completion.
- Angular already has a polling pattern for upload-processing status.

The new feature must extend these patterns instead of replacing them.

## 3. Architecture decision

Use three processes with clear ownership:

1. **Go API** — public API, authentication, upload ownership, job creation, result validation, persistence, and UI-facing endpoints.
2. **Go analysis dispatcher** — claims durable analysis jobs and calls the private Python service outside the user's HTTP request.
3. **Python analysis service** — stateless audio computation. It downloads one object through a short-lived presigned GET URL and returns structured JSON.

The existing final-media worker remains independent so slow model inference cannot delay cover processing, waveform generation, or beat publication.

### Required runtime flow

1. The browser uploads the preview MP3 directly to R2.
2. The browser calls the existing preview confirmation endpoint.
3. Go verifies the R2 object.
4. In the same durable operation, Go creates or reuses an analysis job for the confirmed preview asset.
5. Go returns the file-confirmation response immediately.
6. The Go analysis dispatcher claims the job.
7. The dispatcher creates a short-lived presigned GET URL for only that object.
8. The dispatcher calls `POST /v1/analyze` on the Python service.
9. Python downloads, validates, decodes, and analyzes the MP3.
10. Python returns structured suggestions and confidence scores.
11. Go validates the response against the marketplace taxonomy and saves it.
12. Angular polls the Go API and offers the results to the producer.

## 4. Non-goals for version one

- Do not analyze stems archives.
- Do not infer music metadata from the cover image.
- Do not generate final prices or licences.
- Do not make AI analysis mandatory for publishing.
- Do not let Angular call the Python service directly.
- Do not give the Python service permanent broad R2 credentials.
- Do not automatically publish or mutate final beat metadata.
- Do not train a custom model before collecting real acceptance/override data.

## 5. Repository layout

Keep the current repositories and add one new service repository or sibling directory.

```text
projects/
├── blueprint-backend/              Go API, dispatcher, database migrations
├── red-wave-app/                   Angular UI
└── audio-metadata-analysis/        New Python service
```

Suggested Python structure:

```text
audio-metadata-analysis/
├── app/
│   ├── main.py
│   ├── config.py
│   ├── api/
│   │   ├── analyze.py
│   │   └── health.py
│   ├── analysis/
│   │   ├── decoder.py
│   │   ├── technical.py
│   │   ├── tempo.py
│   │   ├── tonality.py
│   │   ├── classifier.py
│   │   └── normalizer.py
│   ├── models/
│   │   ├── requests.py
│   │   └── responses.py
│   └── security/
│       └── service_auth.py
├── tests/
│   ├── fixtures/
│   ├── unit/
│   └── integration/
├── Dockerfile
├── pyproject.toml
├── .env.example
└── README.md
```

## 6. Database design

Create a new migration after the current latest migration. Do not add AI state to `spec_processing_jobs`; final media processing and early form assistance have different lifecycles.

Suggested table:

```sql
CREATE TABLE spec_audio_analysis_jobs (
    id UUID PRIMARY KEY,
    session_id UUID NOT NULL
        REFERENCES spec_upload_sessions(id) ON DELETE CASCADE,
    asset_id UUID NOT NULL
        REFERENCES spec_upload_assets(id) ON DELETE CASCADE,

    status VARCHAR(20) NOT NULL DEFAULT 'queued'
        CHECK (status IN (
            'queued', 'processing', 'completed', 'failed', 'superseded'
        )),
    stage VARCHAR(40),
    attempts INTEGER NOT NULL DEFAULT 0,

    source_etag TEXT NOT NULL,
    result JSONB,
    model_version VARCHAR(100),
    error_code VARCHAR(80),
    error_message TEXT,

    worker_id VARCHAR(120),
    locked_at TIMESTAMPTZ,
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    UNIQUE (asset_id)
);

CREATE INDEX idx_spec_audio_analysis_claim
    ON spec_audio_analysis_jobs (status, created_at)
    WHERE status IN ('queued', 'processing');
```

Before implementing, decide whether failed jobs may be manually retried using the same row or whether retry creates a new job. The recommended first version reuses the same job, increments `attempts`, and clears transient error fields.

## 7. Go domain and repository changes

Add domain types for:

- Analysis job status
- Analysis stage
- Analysis job
- Analysis result
- BPM suggestion and alternatives
- Key suggestion
- Ranked suggestion values
- Analysis repository interface

Repository operations should include:

```go
Enqueue(ctx, sessionID, assetID, sourceETag)
GetForProducer(ctx, sessionID, producerID)
ClaimNext(ctx, workerID)
Heartbeat(ctx, jobID, workerID)
Complete(ctx, jobID, workerID, assetID, sourceETag, result)
Fail(ctx, jobID, workerID, code, message, retryable)
RequeueStale(ctx, staleBefore)
SupersedeForReplacedAsset(ctx, sessionID, currentAssetID)
```

Completion must be fenced. Save a result only if:

- The job is still owned by the completing worker.
- The job is still processing.
- The asset is still the current `preview` asset for the upload session.
- The source ETag still matches.

## 8. Upload confirmation integration

At the end of successful preview confirmation:

```text
verify object
persist verified size/content type/ETag
enqueue analysis job for preview asset
return confirmation response
```

Requirements:

- Enqueue only for `preview`, not image, WAV, or stems in version one.
- Enqueue must be idempotent for the same asset ID.
- Failure to enqueue should be observable, but the upload itself must remain usable.
- When a preview is replaced, any old queued result must become unusable.
- A late Python response for an old asset must be discarded.

Prefer making object verification and job insertion one database transaction if repository boundaries allow it safely. If that is too invasive for the first version, make enqueue idempotent and provide a repair path for verified preview assets without jobs.

## 9. Python service contract

### Request

```http
POST /v1/analyze
Authorization: Bearer <internal-service-token>
Idempotency-Key: <analysis-job-id>
Content-Type: application/json
```

```json
{
  "job_id": "uuid",
  "asset_id": "uuid",
  "audio_url": "short-lived-presigned-get-url",
  "source": {
    "file_name": "beat.mp3",
    "content_type": "audio/mpeg",
    "size_bytes": 8453120,
    "etag": "source-etag"
  },
  "taxonomy": {
    "genres": ["TRAP", "DRILL", "R&B", "EXPERIMENTAL", "HOUSE", "LO-FI", "HIP-HOP", "POP", "TECH", "AMBIENT"],
    "moods": ["Moody", "Dark", "Cinematic", "Dreamy", "Aggressive", "Soulful", "Melancholic"],
    "instruments": ["Piano", "Guitar", "Drums", "Synth", "Bass", "Strings", "Brass", "808", "Vocal"]
  },
  "options": {
    "detect_bpm": true,
    "detect_key": true,
    "detect_genre": true,
    "detect_moods": true,
    "detect_instruments": true,
    "max_tags": 3
  }
}
```

### Response

```json
{
  "job_id": "uuid",
  "model_version": "waveyard-analysis-v1",
  "processing_time_ms": 18320,
  "audio": {
    "duration_seconds": 164.8,
    "sample_rate": 44100,
    "channels": 2
  },
  "bpm": {
    "value": 142,
    "confidence": 0.91,
    "alternatives": [71, 142]
  },
  "key": {
    "value": "F# MINOR",
    "confidence": 0.78
  },
  "genre": {
    "value": "DRILL",
    "confidence": 0.84,
    "candidates": [
      { "value": "DRILL", "confidence": 0.84 },
      { "value": "TRAP", "confidence": 0.68 }
    ]
  },
  "moods": [
    { "value": "Dark", "confidence": 0.87 },
    { "value": "Aggressive", "confidence": 0.73 }
  ],
  "instruments": [
    { "value": "808", "confidence": 0.89 },
    { "value": "Drums", "confidence": 0.85 }
  ],
  "tags": ["dark", "drill", "sliding-808"]
}
```

### Error response

```json
{
  "error": {
    "code": "AUDIO_DECODE_FAILED",
    "message": "The supplied object could not be decoded as audio.",
    "retryable": false
  }
}
```

## 10. Python analysis pipeline

Implement the pipeline incrementally:

### Stage A — Secure ingestion

- Validate the URL scheme and reject unexpected protocols.
- Download with a strict timeout and maximum byte limit.
- Do not log the presigned URL.
- Store input only in an isolated temporary directory.
- Confirm that the downloaded size matches the request.
- Delete temporary files in a `finally` block.

### Stage B — Decode and technical metadata

- Use `ffprobe` for container/stream metadata.
- Use `ffmpeg` to decode to a known PCM format.
- Reject files with no valid audio frames.
- Apply process time, CPU, memory, and output-size limits.
- Extract ID3 metadata as a high-confidence optional input.

### Stage C — BPM

- Analyze more than the intro.
- Return the primary BPM, confidence, and half/double alternatives.
- Normalize candidates to the backend range of 60–300 BPM.
- Preserve raw detector output for debugging, but do not expose unnecessary internals publicly.

### Stage D — Key and scale

- Estimate pitch-class/chroma information.
- Return major/minor plus confidence or tonal strength.
- Convert flats to the backend's sharp-only vocabulary.
- Return no suggestion when confidence is below the selected threshold.

### Stage E — Genre, mood, and instruments

- Start with a zero-shot audio/text embedding model against the exact backend taxonomy.
- Use multiple descriptive prompts per class and average their text embeddings.
- Analyze several representative windows instead of only the first seconds.
- Aggregate scores across windows.
- Treat similarity scores as model scores, not calibrated probabilities, until calibration is completed.

### Stage F — Normalization

- Never return labels outside the request taxonomy.
- Limit tags to three.
- Deduplicate labels case-insensitively.
- Clamp confidence values to `[0, 1]`.
- Include a stable model/prompt version.

## 11. Go dispatcher

Add a new command, preferably `cmd/analysis-dispatcher`.

Responsibilities:

- Claim one durable analysis job.
- Maintain a heartbeat while the Python request is running.
- Create a short-lived presigned GET URL for the verified preview object.
- Call Python using an internal service token.
- Set explicit connection and total-request timeouts.
- Parse a size-limited response body.
- Strictly validate the Python response.
- Normalize values to Go domain types.
- Complete the job using worker fencing.
- Retry only retryable failures.
- Requeue stale jobs.

Initial retry policy:

```text
Maximum attempts: 3
Attempt 1: immediate
Attempt 2: after 15 seconds
Attempt 3: after 60 seconds
Total Python request timeout: 3–5 minutes
Presigned GET TTL: 10–15 minutes
```

Do not log authorization headers, service tokens, or presigned URLs.

## 12. Go API for Angular

Add:

```http
GET /spec-uploads/{upload_id}/analysis
```

The endpoint must:

- Require an authenticated producer.
- Verify upload-session ownership.
- Return only the current preview asset's analysis.
- Return `not_started`, `queued`, `processing`, `completed`, or `failed`.
- Include a safe stage label during processing.
- Exclude internal worker IDs, stack traces, URLs, and raw model internals.

Optional later endpoint:

```http
POST /spec-uploads/{upload_id}/analysis/retry
```

Do not implement manual retry until the normal failure flow and idempotency are tested.

## 13. Angular implementation

Add:

- Analysis request/response DTOs.
- An analysis repository/service.
- Polling that begins when preview upload confirmation completes.
- Poll cancellation when the preview is replaced or the component is destroyed.
- A visible queued/processing/completed/failed state.
- Suggestion cards with confidence.
- Per-field Apply actions.
- An Apply All action.

Field application rules:

- High-confidence suggestion + empty untouched field: may auto-apply.
- User-edited field: never overwrite; show suggestion separately.
- Low-confidence result: do not auto-apply.
- Mood/instrument results: suggest additions instead of replacing selections.
- Failed analysis: show a non-blocking message and retain manual form entry.

Revisit the current defaults:

- Key currently defaults to `E MINOR`.
- Moods currently default to `Moody` and `Dark`.

Either make them empty or separately track whether the producer actually selected them. Otherwise, AI results cannot distinguish defaults from user intent.

## 14. Model and product evaluation

Before enabling auto-apply, prepare a private evaluation set of at least 100 representative beats with manually verified:

- BPM
- Key
- Primary genre
- Moods
- Instruments

Include difficult distinctions:

- 70 versus 140 BPM
- Trap versus Drill
- Trap versus Hip-hop
- House versus Tech
- Bass versus 808
- Vocal tag versus actual vocal performance
- Major/minor ambiguity
- Intros with silence, effects, or producer tags

Track:

- Exact BPM accuracy and acceptable tolerance accuracy
- Half/double-tempo error rate
- Key accuracy and relative-major/minor confusion
- Top-1 and top-3 genre accuracy
- Precision/recall for moods and instruments
- Producer acceptance and override rates
- P50/P95 analysis latency

## 15. Testing strategy

### Go tests

- Enqueue is idempotent.
- Only preview confirmation creates analysis jobs.
- Replacing the preview supersedes the old result.
- Job claiming is safe with multiple dispatchers.
- Heartbeat prevents premature reclaim.
- Stale jobs are requeued.
- A late worker cannot complete another worker's lease.
- An old asset/ETag result cannot be saved.
- Python response validation rejects unknown labels and invalid ranges.
- Producer A cannot read Producer B's analysis.
- AI failure does not prevent upload completion or manual publishing.

### Python tests

- Authentication is required.
- Oversized downloads are rejected.
- Download and decode timeouts work.
- Invalid/corrupt audio returns a non-retryable error.
- Temporary files are deleted after success and failure.
- BPM normalization handles half/double candidates.
- Flat keys map to supported sharp keys.
- Unknown labels never escape normalization.
- Model objects load once at startup.
- Repeated idempotency keys return consistent behavior.

### Angular tests

- Polling starts after preview confirmation.
- Polling stops after completion, failure, replacement, or destroy.
- Untouched fields can receive suggestions.
- Dirty fields are never overwritten.
- Apply and Apply All work correctly.
- Low-confidence suggestions do not auto-apply.
- Analysis failure does not disable Publish.

### End-to-end tests

- Upload preview, receive analysis, apply results, and publish.
- Replace preview during analysis and confirm only the new result appears.
- Stop Python temporarily, verify retries, restore it, and complete.
- Publish before analysis completes.
- Refresh the browser while analysis is running and resume status display.

## 16. Observability

Add structured logs and metrics for:

- Queued analysis-job count
- Oldest queued-job age
- Jobs completed/failed/retried/superseded
- Python response status and latency
- Analysis stage duration
- Model version
- P50/P95 end-to-end time
- Result acceptance/override rate

Never include raw audio, service tokens, authorization headers, or presigned URLs in logs.

## 17. Deployment

Deploy three independently scalable services:

```text
Go API
Go final-media worker
Go analysis dispatcher
Python analysis service
```

Initial Python settings:

```text
Instances: 1
Analysis concurrency per instance: 1
GPU: none initially
Health checks: /health/live and /health/ready
Model loading: application startup
```

Add configuration variables without committing secrets:

```text
ANALYSIS_SERVICE_URL
ANALYSIS_SERVICE_TOKEN
ANALYSIS_REQUEST_TIMEOUT
ANALYSIS_PRESIGN_TTL
ANALYSIS_MAX_ATTEMPTS
ANALYSIS_WORKER_ID
ANALYSIS_WORKER_POLL_INTERVAL
ANALYSIS_WORKER_LEASE_DURATION
MODEL_VERSION
MAX_AUDIO_DOWNLOAD_BYTES
```

## 18. Recommended implementation order

1. Freeze the Go/Python JSON contract.
2. Create the migration and Go domain/repository layer.
3. Enqueue a job after verified preview confirmation.
4. Expose analysis status through Go using placeholder results.
5. Scaffold Python with authentication and health checks.
6. Implement secure download, FFmpeg validation, and technical metadata.
7. Implement BPM and key.
8. Add the Go dispatcher and real Python call.
9. Add Angular polling and suggestion UI.
10. Add genre, mood, instrument, and tags.
11. Build the evaluation dataset and calibrate thresholds.
12. Add deployment hardening, metrics, and failure drills.

## 19. Definition of done

The feature is complete when:

- A confirmed preview automatically creates exactly one durable analysis job.
- The upload confirmation request does not wait for model inference.
- The Python service is private and authenticated.
- Python can analyze the preview without permanent broad R2 credentials.
- Results are saved only for the current asset and matching ETag.
- BPM, key, genre, moods, instruments, and tags use the backend's valid vocabulary.
- Angular displays progress and suggestions while the form remains editable.
- User-edited fields are never overwritten.
- Failure or low confidence always falls back to manual entry.
- Retries, stale jobs, preview replacement, browser refresh, and Python downtime are tested.
- Logs and metrics reveal queue health and latency without exposing sensitive URLs or audio.
- The selected libraries and model weights are legally approved for commercial deployment.

