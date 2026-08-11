# Audio Metadata Analysis — Progress Tracker

Owner: Saransh  
Started: 2026-07-31  
Last updated: 2026-08-07  
Implementation plan: `docs/audio-metadata-analysis-plan.md`

## Progress summary

One checked implementation task equals one progress unit. Planning documents are not counted as implementation.

```text
Completed implementation tasks: 5
Total implementation tasks:     55
Remaining implementation tasks: 50
Overall completion:              9.1%
```

Formula:

```text
completion percentage = completed tasks / 55 × 100
```

Update the summary whenever checkboxes change. Add a dated note to the change log for every meaningful implementation session.

## Existing prerequisites — not included in the 55 tasks

- [x] Direct Angular-to-R2 uploads exist.
- [x] Go verifies uploaded objects using R2 metadata.
- [x] Upload sessions and upload assets exist in PostgreSQL.
- [x] Durable PostgreSQL processing-job patterns exist.
- [x] A background Go media worker exists.
- [x] Angular has a reusable upload-status polling pattern.
- [x] Architecture and implementation plan documented.
- [x] Progress tracker created.

## Phase summary

| Phase | Scope | Tasks | Done | Remaining | Status |
|---|---|---:|---:|---:|---|
| P0 | Decisions and foundation | 5 | 1 | 4 | In progress |
| P1 | Database and Go domain | 7 | 0 | 7 | Not started |
| P2 | Go application and public API | 8 | 0 | 8 | Not started |
| P3 | Go dispatcher and Python client | 7 | 0 | 7 | Not started |
| P4 | Python analysis service | 12 | 4 | 8 | In progress |
| P5 | Angular integration | 8 | 0 | 8 | Not started |
| P6 | Testing, deployment, and rollout | 8 | 0 | 8 | Not started |
| **Total** |  | **55** | **5** | **50** | **9.1%** |

## P0 — Decisions and foundation: 1/5

- [ ] **P0-01** Freeze the version-one scope and non-goals.
- [ ] **P0-02** Freeze the Go-to-Python request, success response, and error response schemas.
- [ ] **P0-03** Choose commercially approved libraries and model weights for BPM, key, and classification.
- [x] **P0-04** Create the `audio-metadata-analysis` Python service directory/repository and basic project tooling.
- [ ] **P0-05** Assemble an initial labelled evaluation set of representative beats.

Exit criteria:

- Contract examples are agreed and versioned.
- No unresolved commercial licensing question blocks implementation.
- The Python project can install dependencies and run an empty test suite.

## P1 — Database and Go domain: 0/7

- [ ] **P1-01** Write the `spec_audio_analysis_jobs` up migration.
- [ ] **P1-02** Write and test the down migration.
- [ ] **P1-03** Add Go analysis job status, stage, and domain entity types.
- [ ] **P1-04** Add typed Go analysis-result structures with confidence and candidate values.
- [ ] **P1-05** Define the analysis repository interface.
- [ ] **P1-06** Implement PostgreSQL enqueue, get, claim, heartbeat, complete, fail, supersede, and stale-requeue operations.
- [ ] **P1-07** Add repository tests for idempotency, concurrent claims, fencing, stale jobs, and asset replacement.

Exit criteria:

- Migration applies and rolls back cleanly.
- Multiple workers cannot own the same job concurrently.
- An obsolete asset or lost lease cannot save a result.

## P2 — Go application and public API: 0/8

- [ ] **P2-01** Add an application-level analysis service.
- [ ] **P2-02** Enqueue analysis after successful preview confirmation.
- [ ] **P2-03** Ensure image, WAV, and stems confirmations do not enqueue version-one analysis jobs.
- [ ] **P2-04** Make enqueue idempotent and handle enqueue failure without breaking the verified upload.
- [ ] **P2-05** Add the producer-owned `GET /spec-uploads/{id}/analysis` endpoint.
- [ ] **P2-06** Define safe API DTOs for not-started, queued, processing, completed, and failed states.
- [ ] **P2-07** Register the route and wire dependencies through the catalog module.
- [ ] **P2-08** Add handler/service tests for ownership, state mapping, preview replacement, and non-blocking failure.

Exit criteria:

- Confirming a preview produces one durable job and returns without waiting for Python.
- Angular can read only the authenticated producer's current analysis state.

## P3 — Go dispatcher and Python client: 0/7

- [ ] **P3-01** Add presigned GET support for one private R2 object.
- [ ] **P3-02** Implement a strict, authenticated Python HTTP client with bounded request/response sizes and timeouts.
- [ ] **P3-03** Validate and normalize every Python response before persistence.
- [ ] **P3-04** Implement retry classification and attempt limits.
- [ ] **P3-05** Create `cmd/analysis-dispatcher` with worker identity, polling, leases, heartbeats, and graceful shutdown.
- [ ] **P3-06** Add dispatcher tests for success, timeout, retry, permanent failure, lost lease, and stale asset results.
- [ ] **P3-07** Add dispatcher configuration to `.env.example`, Dockerfile/build targets, Makefile, and Docker Compose.

Exit criteria:

- The dispatcher can safely complete a job against a stub Python service.
- Python downtime causes durable retries rather than lost jobs or failed uploads.

## P4 — Python analysis service: 4/12

- [x] **P4-01** Add configuration loading, structured logging, and startup validation.
- [x] **P4-02** Implement `/health/live` and `/health/ready`.
- [x] **P4-03** Implement internal bearer-token authentication.
- [x] **P4-04** Define strict request, success, and error models.
- [ ] **P4-05** Implement bounded audio download without logging the presigned URL.
- [ ] **P4-06** Implement temporary-directory cleanup on success, failure, cancellation, and timeout.
- [ ] **P4-07** Implement `ffprobe` inspection and bounded `ffmpeg` decoding.
- [ ] **P4-08** Extract ID3 and technical audio metadata.
- [ ] **P4-09** Implement BPM detection, confidence, and half/double candidates.
- [ ] **P4-10** Implement key/scale detection, confidence, and sharp-key normalization.
- [ ] **P4-11** Implement genre, mood, and instrument classification against the supplied taxonomy.
- [ ] **P4-12** Implement tag normalization, model-version reporting, unit tests, integration tests, and Docker packaging.

Exit criteria:

- The service handles valid, corrupt, oversized, and timed-out audio safely.
- One request returns schema-valid results without labels outside the supplied taxonomy.
- Models load once at startup.

## P5 — Angular integration: 0/8

- [ ] **P5-01** Add analysis API request and response DTOs.
- [ ] **P5-02** Add the Angular analysis service/repository.
- [ ] **P5-03** Start polling after preview upload confirmation completes.
- [ ] **P5-04** Stop polling on terminal state, preview replacement, navigation, and component destruction.
- [ ] **P5-05** Add queued, processing, completed, and failed UI states.
- [ ] **P5-06** Add per-field Apply and Apply All suggestion controls with confidence display.
- [ ] **P5-07** Protect dirty fields and revise default key/mood behavior so defaults are not mistaken for user intent.
- [ ] **P5-08** Add Angular tests for polling, replacement, dirty-field protection, low confidence, failure, and publish independence.

Exit criteria:

- The producer can continue editing while analysis runs.
- AI never overwrites a producer-edited field.
- Analysis failure never disables manual publishing.

## P6 — Testing, deployment, and rollout: 0/8

- [ ] **P6-01** Add end-to-end coverage for upload, analysis, apply suggestions, and publish.
- [ ] **P6-02** Test preview replacement while analysis is running.
- [ ] **P6-03** Test Python downtime, retries, recovery, and exhausted attempts.
- [ ] **P6-04** Add queue-depth, queue-age, latency, retry, failure, superseded, and model-version metrics.
- [ ] **P6-05** Add log redaction checks for tokens, authorization headers, audio content, and presigned URLs.
- [ ] **P6-06** Deploy one analysis dispatcher and one single-concurrency Python instance in a non-production environment.
- [ ] **P6-07** Evaluate accuracy and latency on the labelled beat set and calibrate confidence thresholds.
- [ ] **P6-08** Complete security/licensing review, failure drills, rollout checklist, and production enablement.

Exit criteria:

- Operational dashboards reveal job health and latency.
- Accuracy thresholds are based on representative beats rather than guesses.
- The feature can be disabled without affecting the existing manual upload flow.

## Current blockers

None recorded. The first unresolved decision is **P0-03: library and model licensing approval**.

## Decisions log

| Date | Decision | Reason |
|---|---|---|
| 2026-07-31 | Use a separate Python analysis service. | Keeps audio/ML dependencies out of the Go API. |
| 2026-07-31 | Go remains the system of record. | Authentication, taxonomy, persistence, retries, and stale-result protection stay centralized. |
| 2026-07-31 | Call Python from a background Go dispatcher. | Upload confirmation must not wait for model inference. |
| 2026-07-31 | Trigger from verified preview confirmation. | Results can arrive while the producer is still filling the form. |
| 2026-07-31 | Use a short-lived presigned GET URL. | Avoids giving Python permanent broad R2 credentials. |

## Implementation session log

| Date | Tasks completed | Notes | Next task |
|---|---|---|---|
| 2026-07-31 | Planning documents only | No application code implemented. | P0-01 |
| 2026-08-02 | P0-04 | FastAPI project runs on Python 3.12; health endpoints work; 2 tests and Ruff pass. Pylance still needs the project `.venv` selected in VS Code. | P0-01/P0-02 |
| 2026-08-05 | P4-02, P4-04 | Health routes and strict analysis request/response contracts verified; 7 tests and Ruff pass. | P4-01/P4-03 |
| 2026-08-07 | P4-03 | Added application-scoped settings, bearer-token protection, injected test settings, shared request builder, and authentication tests; 9 tests and Ruff pass. | Complete structured logging for P4-01 |
| 2026-08-07 | P4-01 | Added validated log-level settings, secret-safe validation errors, JSON logging, lifecycle events, and logging/configuration tests; 12 tests and Ruff pass. | P4-05 bounded audio download |
