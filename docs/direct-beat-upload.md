# Direct beat upload and durable processing

Beat creation no longer sends a large multipart request through the Go API.
The browser uploads to private object storage, and a separate worker processes
the upload from a durable PostgreSQL job.

## Runtime flow

1. When the producer selects the first valid file, the authenticated frontend
   calls `POST /spec-uploads` with an empty JSON object. The API creates an
   expiring draft upload session.
2. For each selected file, the frontend calls
   `POST /spec-uploads/{upload_id}/files` with only that file's kind, name,
   content type, and size. The API replaces any earlier draft asset of that
   kind and returns a server-generated, presigned `PUT` instruction for a
   random `incoming/` object key.
3. The browser immediately uploads that file directly to R2/S3 using the exact
   headers in the instruction and shows progress for that file. Each signature
   binds the exact file size. API cookies, bearer tokens, and storage
   credentials are never sent to storage.
4. After the storage `PUT` succeeds, the frontend calls
   `POST /spec-uploads/{upload_id}/files/{asset_id}/complete`. The API checks
   the current asset with `HEAD`, verifies its size and content type, and marks
   it ready. Re-selecting a file gets a new asset ID and upload URL, so a late
   response from the replaced upload cannot confirm the replacement.
5. Publish saves the final form values with
   `PUT /spec-uploads/{upload_id}/metadata`, then calls
   `POST /spec-uploads/{upload_id}/complete`. No file bytes are uploaded during
   Publish.
6. The API re-verifies every server-known object with `HEAD`, then atomically
   inserts the processing spec, its relations, and a queued PostgreSQL job.
7. `cmd/worker` claims the job with `FOR UPDATE SKIP LOCKED`, validates the
   actual file bytes, derives MP3 duration and waveform data, resizes the cover,
   and promotes assets to final keys.
8. One database transaction publishes the stable object URLs and changes the
   spec to `completed`. Existing catalog, playback, and purchased-download
   responses continue to sign those same stable URLs.

The frontend can select and upload the cover, preview MP3, master WAV, and
stems independently or concurrently. Publish stays disabled until all four
server-confirmed file states are ready.

`Content-Type` is sent and stored as object metadata, then checked by the API.
It is not signature-bound by the current AWS SDK presign path, so the server
still performs the `HEAD` verification before accepting the asset.

The worker never writes upload data to the local filesystem. Preview, WAV, and
stems promotion uses object-storage copy operations; cover and waveform
processing stream from storage.

In this phase the worker validates ZIP/RAR signatures, a meaningful minimum
archive size, and the first ZIP local-file header, but it does not extract or
malware-scan stems archives. Add archive scanning as a separate worker stage
before accepting untrusted marketplace uploads at larger scale.

## Required processes

Run both processes against the same PostgreSQL database and object-storage
bucket:

```powershell
make run
make run-worker
```

The Docker image contains both `./server` and `./worker`. Docker Compose starts
both services. In a hosted environment, create one web service with
`./server` and one background worker with `./worker`.

If the API runs without a worker, completed browser uploads remain queued and
never become playable.

## Cloudflare R2 CORS

API CORS and bucket CORS are separate. Configure the R2 bucket to allow direct
browser uploads from the real frontend origins. A starting rule is:

```json
[
  {
    "AllowedOrigins": [
      "http://localhost:4200",
      "https://qa.waveyard.studio",
      "https://waveyard.studio"
    ],
    "AllowedMethods": ["PUT", "GET", "HEAD"],
    "AllowedHeaders": ["Content-Type"],
    "ExposeHeaders": ["ETag"],
    "MaxAgeSeconds": 3600
  }
]
```

Remove origins that are not used and add the exact deployed frontend origin.
Keep the bucket private.

Use the R2 S3 API endpoint for `S3_ENDPOINT` and `S3_PRESIGN_ENDPOINT`:

```env
S3_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
S3_PRESIGN_ENDPOINT=https://<account-id>.r2.cloudflarestorage.com
```

Do not use an R2 custom domain as `S3_PRESIGN_ENDPOINT`; custom domains cannot
serve R2 S3 presigned requests. `S3_PUBLIC_ENDPOINT` remains optional and, in
the current storage adapter, must be a path-style endpoint where
`/{bucket}/{key}` resolves to the object.

## Quarantine cleanup

Upload URLs can be reused until they expire, so they only target random
`incoming/` keys. Published assets live at different deterministic keys that
were never presigned for upload.

Configure object lifecycle rules that expire both the `incoming/` and
`processing/` prefixes after a short retention period (for example, two days).
`incoming/` holds abandoned browser uploads; `processing/` holds temporary
cover sources used by the worker. This removes leftovers from interrupted or
ambiguously committed processing without risking published assets.

## Operational checks

- Apply migration `000036` before accepting upload sessions.
- Confirm both API and worker have identical database and S3 credentials.
- Confirm `USE_S3=true`; local filesystem storage cannot provide genuine
  browser-to-object-storage upload URLs.
- Watch queued/failed job counts and worker logs during deployment.
- Test a full upload, processing completion, catalog fetch, audio range
  playback, and purchased WAV/stems download before enabling production traffic.
