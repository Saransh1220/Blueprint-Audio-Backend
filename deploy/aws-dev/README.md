# AWS Dev Server (on/off via Telegram)

Cheap, on-demand dev environment for Blueprint (backend) + Red Wave / Waveyard (UI).

```
Telegram /start /stop /status
        │  (webhook)
        ▼
 Lambda Function URL ──► EC2 start/stop          EventBridge (30 min) ──► auto-stop after 4h
                              │
              ┌───────────────▼─────────────────┐
              │ EC2 t4g.small (stopped = $0)    │
              │  docker: api + worker +         │
              │          postgres + cloudflared │
              │  EBS 20GB (DB data persists)    │
              └───────────────┬─────────────────┘
                              │ Cloudflare Tunnel (free, stable HTTPS)
                    https://api-dev.waveyard.studio
                              ▲
   S3 (private) + CloudFront (always on, ~$0) ── https://dev.waveyard.studio  (Angular UI)
   Cloudflare R2 (dev bucket) for audio files · Redis OFF · Neon not needed
```

## Why this design

| Piece | Choice | Why |
|---|---|---|
| Compute | One EC2 `t4g.small` (2 GB ARM), **stop/start** | A stopped instance bills **no compute**. Fargate/App Runner/ECS can't scale to zero this simply. |
| DB | Postgres container on the same EC2, data on EBS | Free, survives stop/start. RDS charges for storage while stopped and auto-restarts after 7 days. |
| Worker | Separate `./worker` container, same box | Free here (unlike Render). Same image, already supported by the Dockerfile. |
| Redis | Off (`REDIS_ENABLED=false`) | Same as your QA. |
| HTTPS / stable URL | Cloudflare Tunnel | Instance public IP changes on every start; an idle Elastic IP costs money. Tunnel needs no inbound ports at all. |
| UI | S3 + CloudFront | Static, effectively free, always on (it's just files). |
| Files | Keep Cloudflare R2 (separate dev bucket) | 10 GB free, no code change. |
| On/off | Telegram bot → Lambda Function URL | Lambda/Function URL/EventBridge are free at this volume. WhatsApp needs Meta business verification, so skip it. |

### Cost (approximate, ap-south-1, check the AWS pricing page)
- **Off:** EBS 20 GB gp3 ≈ $1.8/mo + pennies for S3/CloudFront/Lambda ≈ **~$2/mo**.
- **On:** t4g.small ≈ $0.02/hr + public IPv4 $0.005/hr ≈ **$0.025/hr**. 40 hrs/month ≈ $1.
- Your $100 credits cover this for years. Note: new AWS accounts on the credit-based free plan have a 6-month window, then you must upgrade to a paid plan (credits still apply until used/expired).

---

## Step 0 – Safety first (5 min)
1. Billing → **Budgets** → create a monthly cost budget of **$5** with email alerts at 50/100%.
2. Don't use the root user. Create an IAM user/Identity Center user with admin, enable MFA, and run `aws configure` (or `aws configure sso`) on your Windows machine. Pick one region and use it everywhere (examples use `ap-south-1`).

## Step 1 – EC2 instance
1. **IAM → Roles → Create role** → trusted entity EC2 → attach `AmazonSSMManagedInstanceCore` → name `blueprint-dev-ec2`. (This lets you shell in via Session Manager; no SSH, no open ports.)
2. **EC2 → Launch instance**
   - Name `blueprint-dev`
   - AMI: **Amazon Linux 2023, arm64**
   - Type: **t4g.small**
   - Key pair: none
   - Network: default VPC, auto-assign public IP **enabled**
   - Security group: **new, with NO inbound rules**
   - Storage: **20 GiB gp3**
   - Advanced → IAM instance profile `blueprint-dev-ec2`
   - Advanced → User data: paste [`bootstrap.sh`](bootstrap.sh)
3. Note the **Instance ID** (`i-0123...`). Wait ~3 min for the bootstrap to finish.

## Step 2 – Cloudflare Tunnel (stable API URL)
Your `waveyard.studio` DNS must be on Cloudflare (it already is if QA Pages uses a custom domain there).
1. Cloudflare dashboard → **Zero Trust → Networks → Tunnels → Create tunnel** → Cloudflared → name `blueprint-dev`.
2. Copy the **tunnel token**.
3. **Public hostname:** `api-dev.waveyard.studio` → Service **HTTP** → `api:8080`.

## Step 3 – Put the backend on the instance
EC2 console → select instance → **Connect → Session Manager**. Then:

```bash
sudo -i
# private repo: create a fine-grained GitHub token with read-only "Contents" on this repo
git clone -b dev https://<TOKEN>@github.com/Saransh1220/Blueprint-Audio-Backend.git /opt/blueprint-backend
cd /opt/blueprint-backend
cp deploy/aws-dev/.env.dev.example .env.dev
nano .env.dev           # fill in every value (see notes below)
chmod 600 .env.dev
docker compose -f deploy/aws-dev/docker-compose.dev.yml --env-file .env.dev up -d --build
docker compose -f deploy/aws-dev/docker-compose.dev.yml logs -f api   # expect: migrations ok, "Redis disabled", "Server starting on port 8080"
curl -s localhost:8080/health
```
(the first build takes several minutes on 2 vCPU; that's normal).

Open `https://api-dev.waveyard.studio/health` from your browser to confirm the tunnel.

**`.env.dev` notes**
- `ENV=production` is required: the refresh cookie only becomes `Secure; SameSite=None` in production mode.
- `ALLOWED_ORIGINS` / `APP_BASE_URL` = your UI origin exactly (`https://dev.waveyard.studio`), no trailing slash.
- Use **R2 dev bucket** and add the UI origin to that bucket's **CORS** rules (allow `PUT, GET, HEAD`, all headers). Uploads go browser → R2 directly via presigned URLs, so missing CORS = failed uploads.
- Razorpay/Dodo: test keys only. Dodo/Razorpay webhooks can only reach you while the server is on.
- Generate secrets: `openssl rand -base64 32`.

**Redeploy later:** Session Manager →
```bash
cd /opt/blueprint-backend && git pull origin dev && docker compose -f deploy/aws-dev/docker-compose.dev.yml --env-file .env.dev up -d --build
```

## Step 4 – UI on S3 + CloudFront

### 4a. Build with a dev API URL
Add `red-wave-app/src/environments/environment.dev.ts` (copy of `environment.prod.ts`, with `apiUrl: 'https://api-dev.waveyard.studio'` and your **test** Razorpay key), and add a `dev` build configuration in `angular.json` next to `production` (same options, but `fileReplacements` → `environment.dev.ts`). Then:

```powershell
cd red-wave-app
npm ci
npx ng build --configuration dev
```
Output: `dist/blueprint-audio/browser`.

### 4b. Bucket + CDN (console)
1. **S3** → create bucket `waveyard-dev-ui-<random>`, Block all public access **ON**.
2. **ACM** (region **us-east-1**, required for CloudFront) → request cert for `dev.waveyard.studio` → DNS validation → add the CNAME it shows in Cloudflare DNS (**DNS only**, grey cloud).
3. **CloudFront → Create distribution**
   - Origin: the S3 bucket, **Origin access control (OAC)** → let it update the bucket policy
   - Default root object: `index.html`
   - Viewer protocol: redirect HTTP→HTTPS; price class: **Use only North America and Europe** (cheapest; fine for dev)
   - Alternate domain: `dev.waveyard.studio`, your ACM cert
   - **Error pages** (SPA routing): 403 → `/index.html` → 200, and 404 → `/index.html` → 200
4. In Cloudflare DNS add `dev` CNAME → `dxxxxxxxx.cloudfront.net` (**DNS only**).

### 4c. Upload (repeat per UI deploy)
```powershell
aws s3 sync dist/blueprint-audio/browser s3://waveyard-dev-ui-<random> --delete
aws cloudfront create-invalidation --distribution-id <ID> --paths "/*"
```

### 4d. Google sign-in
Google Cloud console → OAuth client → add `https://dev.waveyard.studio` to **Authorized JavaScript origins** (that's why a custom domain beats the raw `cloudfront.net` URL).

## Step 5 – Telegram on/off control

### 5a. Bot
1. In Telegram, talk to **@BotFather** → `/newbot` → copy the **bot token**.
2. Message your new bot once, then open `https://api.telegram.org/bot<TOKEN>/getUpdates` and copy `message.chat.id` (your **chat id**).
3. Generate a random **webhook secret** (letters/digits only).

### 5b. Lambda (console)
1. **IAM → Policies → Create** (JSON), name `blueprint-dev-power`:
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {"Effect":"Allow","Action":"ec2:DescribeInstances","Resource":"*"},
    {"Effect":"Allow","Action":["ec2:StartInstances","ec2:StopInstances"],
     "Resource":"arn:aws:ec2:<region>:<account-id>:instance/<instance-id>"}
  ]
}
```
2. **Lambda → Create function** → Python 3.12, name `blueprint-dev-power`, new role → then attach `blueprint-dev-power` to that role. Paste [`lambda/handler.py`](lambda/handler.py) as `lambda_function.py`, then change handler to `lambda_function.lambda_handler` (or paste into the default file and keep the default handler name).
3. Configuration → **General: Timeout 15 s**.
4. Configuration → **Environment variables**:
   `INSTANCE_ID`, `BOT_TOKEN`, `WEBHOOK_SECRET`, `ALLOWED_CHAT_IDS`, `MAX_UPTIME_HOURS=4`, `DEV_API_URL=https://api-dev.waveyard.studio`, `DEV_UI_URL=https://dev.waveyard.studio`
5. Configuration → **Function URL → Create**, auth type **NONE** (Telegram can't sign requests; the secret-token header + chat-id allowlist protect it). Copy the URL.
6. **EventBridge → Schedules** (or add trigger → EventBridge rule): rate `30 minutes` → target this Lambda. This is the "you forgot to turn it off" safety net.

### 5c. Point Telegram at it
```powershell
curl.exe -s "https://api.telegram.org/bot<TOKEN>/setWebhook" `
  -d "url=<LAMBDA_FUNCTION_URL>" -d "secret_token=<WEBHOOK_SECRET>"
```

### 5d. Use it
| Message | Effect |
|---|---|
| `/start` | Starts EC2. About 1-2 min later the instance pings you "Dev server is UP". |
| `/status` | State, uptime, URLs |
| `/stop` | Stops EC2 (compute billing stops) |
| (automatic) | Stops after `MAX_UPTIME_HOURS` |

Fallback without Telegram:
```powershell
aws ec2 start-instances --instance-ids i-xxxx
aws ec2 stop-instances  --instance-ids i-xxxx
```

## Gotchas
- **Data:** DB lives on EBS and persists across stop/start. Do **not** terminate the instance. Take an EBS snapshot (or `pg_dump` to S3) before risky changes.
- **Deleting the EC2** would delete the DB (root volume). Enable termination protection on the instance.
- **Cold start:** ~1-2 min. The API container restarts automatically (`restart: unless-stopped` + systemd unit).
- **Cloudflare Tunnel in `docker compose logs cloudflared`** shows "Registered tunnel connection" when healthy.
- **QA stays untouched** (Render/Neon/Pages); dev uses its own DB, bucket and keys.
- **Cheaper still:** if you don't need DB persistence, drop to `t4g.micro`... but 1 GB RAM is tight for building Go in Docker; build once with swap (bootstrap adds 2 GB) or build images elsewhere.
