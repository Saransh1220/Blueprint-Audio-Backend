# AWS Dev Deployment - Notes, Decisions, Status

Branch deployed: `dev` (both `Blueprint-Audio-Backend` and `Blueprint-Audio`), cut from `main`.
AWS account 2590-6919-7434, region **ap-south-1 (Mumbai)**.

## Environments stay separate

| | QA (existing, untouched) | Dev (new) |
|---|---|---|
| UI | Cloudflare Pages, `qa.waveyard.studio` | S3 + CloudFront, `dev.waveyard.studio` |
| API | Render, `api-qa.waveyard.studio` | EC2, `api-dev.waveyard.studio` |
| DB | Neon | Postgres container on the EC2 disk |
| Files | R2 bucket `blueprint-audio-assets` | **separate** R2 bucket `blueprint-audio-assets-dev` + its own API token |
| Worker | embedded in API (no paid worker) | separate `./worker` container |
| Secrets | QA values | own JWT secret, DB password, Razorpay test keys |
| Branch | main | dev |

Nothing in QA was modified: only new DNS name `api-dev` was added.

## Decisions and why

1. **EC2 stop/start instead of ECS/Fargate/App Runner/RDS.** A stopped EC2 bills no compute; the others cannot go to zero cost this simply, and RDS bills storage while stopped and auto-restarts after 7 days.
2. **t4g.small (2 GB, Arm).** t4g.micro (1 GB) is too small to build the Go image. Arm is cheaper; the Dockerfile builds natively on Arm. Price in Mumbai about $0.0112/hr.
3. **Postgres in Docker on the same instance** (data on EBS `/data/pg`). Free and persistent. Neon was the alternative but the point was to run on AWS.
4. **Redis off** (`REDIS_ENABLED=false`), same as QA. No code path needs it.
5. **Worker = separate container** running `./worker` (the only background worker in the code: the media upload processor, a PostgreSQL job queue). Trade-off found in the code: the standalone worker saves notifications to the DB but only the API process broadcasts them over WebSocket, so live "processing finished" pushes will not appear in real time on dev (they show on refresh). To get live pushes, set `WORKER_ENABLED=true` on the API and remove the worker service.
6. **File storage stays on Cloudflare R2 (new dev bucket), not S3.** The code is configured and tested for R2 (`S3_REGION=auto`, presigned URLs). R2 has 10 GB free and no egress fees. S3 would also be near-free but is untested with this code. Needs bucket CORS for `https://dev.waveyard.studio` because uploads go browser to R2 directly.
7. **HTTPS for the API via Cloudflare Tunnel.** The EC2 public IP changes each start and an idle Elastic IP costs about $3.65/month; the browser also requires HTTPS. The tunnel gives a stable `https://api-dev.waveyard.studio` with no open inbound ports. Zero Trust Free plan activated ($0; Cloudflare requires a card on file).
8. **UI on S3 + CloudFront.** Static files, always on, about $0. Custom domain `dev.waveyard.studio` (Google sign-in needs a normal domain as an authorized origin).
9. **On/off via Telegram bot to Lambda Function URL to EC2 start/stop.** WhatsApp needs Meta business verification, so skipped. A 30-minute EventBridge schedule auto-stops the server after 4 hours.
10. **`ENV=production` on dev** so the refresh cookie is `Secure; SameSite=None`.
11. **No SSH, no key pair, security group with zero inbound rules.** Access via Session Manager (IAM role `blueprint-dev-ec2`).

## Cost expectation
Off: about $2/month (20 GB EBS + pennies). On: about $0.02/hr plus IPv4 $0.005/hr.

## Done so far
- [x] `dev` branches created and pushed (backend + UI); deploy kit committed
- [x] IAM role `blueprint-dev-ec2` (AmazonSSMManagedInstanceCore)
- [x] EC2 `i-04294d7d2215806ac` (t4g.small, 20 GB gp3, no inbound, bootstrap user-data)
- [x] Cloudflare Zero Trust Free activated
- [x] Tunnel `blueprint-dev` (id 1a51a80a-04af-4076-be07-b4d245dc7f0c), route `api-dev.waveyard.studio` to `http://api:8080`

- [x] Cloudflare DNS: `api-dev` (tunnel, auto-created) and ACM validation CNAME `_d5fbd8d5...dev` (DNS only). QA records `api-qa`, `qa` untouched
- [x] S3 bucket `waveyard-dev-ui-259069197434` (ap-south-1, private, block public access on)
- [x] ACM certificate for `dev.waveyard.studio` requested in us-east-1 (id 45b237a7-ba33-4815-aee6-b7cc02aaf4a5), status "Pending validation" at time of writing

## Next (assistant, once cert shows Issued)
- [ ] CloudFront distribution: S3 origin with OAC, default root `index.html`, alternate domain `dev.waveyard.studio`, ACM cert, 403/404 to `/index.html` 200, price class North America+Europe (cheapest)
- [ ] Cloudflare DNS: `dev` CNAME to the CloudFront domain (DNS only)
- [ ] Build UI with `ng build --configuration dev`, upload to S3, invalidate CloudFront

## Needs you (secrets are never typed by the assistant)
- [ ] Copy the tunnel token (Cloudflare, Networks, Tunnels, blueprint-dev, Docker tab) into `.env.dev`
- [ ] GitHub read-only token to clone `dev`
- [ ] R2 dev bucket + API token; JWT secret; DB password; Razorpay test keys in `.env.dev`
- [ ] Telegram bot token, chat id, webhook secret

## Status update
- [x] Backend stack running on EC2 (api, worker, postgres, cloudflared). `https://api-dev.waveyard.studio/health` returns 200 and CORS allows `https://dev.waveyard.studio`.
- Gotchas hit: compose needed docker buildx (fixed in bootstrap.sh); `SUPER_ADMIN_EMAIL` must be empty on a fresh DB or the API exits (set it after the first signup); CloudFront wizard preselects paid WAF, choose "Do not enable".
- [x] ACM certificate for `dev.waveyard.studio` is Issued.
- BLOCKED: CloudFront create fails with "Your account must be verified before you can add new CloudFront resources" (same new-account verification as CloudShell).

- DECISION: dev UI hosted on a separate Cloudflare Pages project `waveyard-dev` (production branch `dev`, build `npx ng build --configuration dev`, output `dist/blueprint-audio/browser`, `NODE_VERSION=22`, custom domain `dev.waveyard.studio`) because CloudFront is blocked by account verification. Free, separate from QA's Pages project. Can move to S3 + CloudFront later (bucket and ACM cert already exist).
- [x] API `ALLOWED_ORIGINS` = dev.waveyard.studio + waveyard-dev.pages.dev; R2 dev bucket CORS set (GET/PUT/HEAD).
- [x] Verified: UI returns 200 on both URLs, bundle calls `api-dev.waveyard.studio`, API CORS allows both origins.
- [ ] Telegram on/off (Lambda) pending AWS account verification; use EC2 console or `aws ec2 start/stop-instances` meanwhile.

## Blockers seen
- CloudFront: account verification required (see above). Lambda may hit the same.
- AWS CloudShell unavailable: "account verification in progress" (new account, up to 2 days). May also affect other services.
