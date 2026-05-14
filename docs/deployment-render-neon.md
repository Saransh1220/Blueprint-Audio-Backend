# Blueprint Audio Deployment: Render + Neon

This guide deploys:

- Frontend: Cloudflare Pages
- Backend API: Render Web Service
- Database: Neon Postgres
- File storage: Cloudflare R2
- Redis: disabled for first production deploy

This is the cheapest practical starting setup for little to no users.

## 0. What You Are Creating

Create these services:

1. Neon project: hosted Postgres database.
2. Render Web Service: runs the Go backend from this repo's `Dockerfile`.
3. Cloudflare Pages project: runs `red-wave-app`.

Do not create a Render PostgreSQL database for this setup. Render's hosted DB is separate from the backend and may cost or have limits. Neon is the cheaper/free starting database.

Do not create a Render Redis/Key Value service at first. This backend now supports:

```env
REDIS_ENABLED=false
```

## 1. Before You Start

You need:

- GitHub repo pushed with latest backend code.
- Render account.
- Neon account.
- Cloudflare account.
- R2 credentials already created.
- Razorpay keys.
- Resend key if email is enabled.
- Google OAuth client ID.
- A generated production JWT secret.

Generate a strong JWT secret locally:

```powershell
openssl rand -base64 32
```

If `openssl` is not available, use any secure password generator and make a long random value.

## 2. Create Neon Postgres

1. Go to Neon dashboard.
2. Click **New Project**.
3. Project name: `blueprint-audio-prod`.
4. Choose the nearest region to your users.
5. Create the project.
6. Open the project dashboard.
7. Click **Connect**.
8. Select:
   - Branch: `main` or the default branch Neon created
   - Database: the default database, or create/use `blueprint_audio`
   - Role: the default owner/app role
9. Copy the connection string.

It will look like:

```text
postgresql://USER:PASSWORD@HOST/DB_NAME?sslmode=require
```

For this backend, split that URL into separate env vars:

```env
DB_HOST=HOST_FROM_NEON
DB_PORT=5432
DB_USER=USER_FROM_NEON
DB_PASSWORD=PASSWORD_FROM_NEON
DB_NAME=DB_NAME_FROM_NEON
DB_SSLMODE=require
```

Example:

```text
postgresql://alex:secret@ep-cool-darkness-a1b2c3d4.us-east-2.aws.neon.tech/blueprint_audio?sslmode=require
```

Becomes:

```env
DB_HOST=ep-cool-darkness-a1b2c3d4.us-east-2.aws.neon.tech
DB_PORT=5432
DB_USER=alex
DB_PASSWORD=secret
DB_NAME=blueprint_audio
DB_SSLMODE=require
```

Use the non-pooled Neon host first. It is simpler for a normal long-running Render backend.

## 3. Database Migrations On Neon

The deployed Render backend can run migrations automatically before the server starts. Set this on Render:

```env
AUTO_MIGRATE=true
MIGRATIONS_PATH=db/migrations
```

With this enabled, every new Render deploy runs all pending migrations first. Already-run migrations are skipped by `golang-migrate`, so this is safe for normal deploys.

For the first production deploy, you can either let Render run migrations automatically, or run them manually once from your local machine before deploying.

To run them manually, temporarily set your local `.env` DB values to the Neon values:

```env
DB_HOST=your-neon-host.neon.tech
DB_PORT=5432
DB_USER=your-neon-user
DB_PASSWORD=your-neon-password
DB_NAME=your-neon-db
DB_SSLMODE=require
```

Then run:

```powershell
make migrate-up
```

Check current migration version:

```powershell
make migrate-version
```

After migrations finish, restore your local `.env` back to local Docker Postgres values if you still want local dev to use Docker.

Important: do not run `make migrate-drop` against Neon production. It drops data.

If Render logs show a "dirty" migration state, stop and fix the database manually before deploying again. A dirty state means one migration partially failed and `golang-migrate` is protecting your data from guessing.

## 4. Create Render Backend Service

In Render, create a **Web Service**. Do not create a Static Site, Private Service, Background Worker, Cron Job, Redis, or PostgreSQL service for the backend.

Steps:

1. Go to Render dashboard.
2. Click **New**.
3. Choose **Web Service**.
4. Select your Git provider, usually GitHub.
5. Connect/select the backend repository.
6. Fill the service form:

```text
Name: blueprint-audio-api
Region: choose nearest region, ideally same general region as Neon
Branch: main
Runtime/Language: Docker
Root Directory: leave blank if this repo is only blueprint-backend
Dockerfile Path: Dockerfile
Instance Type: Free for first deploy
Auto-Deploy: Yes
```

If `blueprint-backend` lives inside a bigger monorepo, set:

```text
Root Directory: blueprint-backend
Dockerfile Path: Dockerfile
```

If Render asks for build/start commands with Docker, leave them empty. The `Dockerfile` handles build and startup.

## 5. Add Render Environment Variables

In the Render service creation page, open **Advanced** or after creation open:

```text
Service -> Environment
```

Add these variables.

### Server

Use `PORT=8080` because this repo's `Dockerfile` exposes `8080` and the local config already uses `8080`.

```env
PORT=8080
ENV=production
AUTO_MIGRATE=true
MIGRATIONS_PATH=db/migrations
REDIS_ENABLED=false
```

### Database

Use your Neon values:

```env
DB_HOST=your-neon-host.neon.tech
DB_PORT=5432
DB_USER=your-neon-user
DB_PASSWORD=your-neon-password
DB_NAME=your-neon-db
DB_SSLMODE=require
```

### Auth

```env
JWT_SECRET=your-long-random-production-secret
JWT_EXPIRATION=24h
JWT_REFRESH_EXPIRATION=720h
GOOGLE_CLIENT_ID=your-google-client-id
```

### Frontend Links And CORS

For QA, use your Porkbun domain as the frontend URL:

```text
https://qa.waveyard.studio
```

Set:

```env
APP_BASE_URL=https://qa.waveyard.studio
ALLOWED_ORIGINS=https://qa.waveyard.studio
```

At first, Cloudflare Pages will also give you a temporary URL like:

```text
https://red-wave-app.pages.dev
```

If you want both the custom QA domain and the temporary Pages URL to work, set both origins as a comma-separated list:

```env
APP_BASE_URL=https://qa.waveyard.studio
ALLOWED_ORIGINS=https://qa.waveyard.studio,https://your-cloudflare-pages-url.pages.dev
```

CORS origins must be exact browser origins:

- Include `https://`.
- Do not include a trailing slash.
- Do not include paths like `/api`.
- Separate multiple origins with commas only.

If you later add the production root domain, update both:

```env
APP_BASE_URL=https://waveyard.studio
ALLOWED_ORIGINS=https://waveyard.studio,https://qa.waveyard.studio
```

### Cloudflare R2

Use your real R2 values:

```env
USE_S3=true
S3_ENDPOINT=https://YOUR_ACCOUNT_ID.r2.cloudflarestorage.com
S3_ACCESS_KEY=your-r2-access-key
S3_SECRET_KEY=your-r2-secret-key
S3_BUCKET=blueprint-audio-assets
S3_USE_SSL=true
S3_REGION=auto
S3_PUBLIC_ENDPOINT=
```

If you use a public/custom R2 domain, set:

```env
S3_PUBLIC_ENDPOINT=https://your-public-r2-domain.com
```

### Payments

For real production payments:

```env
RAZORPAY_KEY_ID=rzp_live_your_key_id
RAZORPAY_KEY_SECRET=your_live_secret_key
```

For testing production deploy flow only, you can temporarily use test keys:

```env
RAZORPAY_KEY_ID=rzp_test_...
RAZORPAY_KEY_SECRET=...
```

### Email

If email should work:

```env
EMAIL_ENABLED=true
RESEND_API_KEY=re_your_api_key
EMAIL_FROM=Blueprint <noreply@yourdomain.com>
EMAIL_REPLY_TO=
```

If you do not have email ready yet, disable it for first deploy:

```env
EMAIL_ENABLED=false
RESEND_API_KEY=
EMAIL_FROM=
EMAIL_REPLY_TO=
```

The backend intentionally fails startup when `EMAIL_ENABLED=true` but required email values are missing.

## 6. First Render Deploy

1. Click **Create Web Service**.
2. Render builds the Docker image.
3. Watch logs in the Render dashboard.
4. Successful logs should include:

```text
Running database migrations...
Migrations completed successfully
Redis disabled; running without cache
Server starting on port 8080
```

5. Render gives you a URL like:

```text
https://blueprint-audio-api.onrender.com
```

That is your backend API URL.

### Recommended QA Backend Domain

For QA, add a custom Render domain such as:

```text
https://api-qa.waveyard.studio
```

This is better than using the raw `onrender.com` URL from the browser because the backend sets the refresh token cookie with `SameSite=Strict`. A frontend on `qa.waveyard.studio` and an API on `api-qa.waveyard.studio` are same-site. A frontend on `qa.waveyard.studio` and an API on `blueprint-audio-api.onrender.com` are cross-site, which can break refresh-token cookies.

In Render:

1. Open the backend Web Service.
2. Go to **Settings** or **Custom Domains**.
3. Add:

```text
api-qa.waveyard.studio
```

4. Add the DNS record Render asks for at Cloudflare or Porkbun.

The DNS record will usually be a CNAME similar to:

```text
api-qa -> your-render-service.onrender.com
```

After SSL is ready, use `https://api-qa.waveyard.studio` as the frontend API URL.

## 7. If The Render Deploy Fails

Check these first:

### Database Connection Fails

Usually one of these is wrong:

```env
DB_HOST
DB_USER
DB_PASSWORD
DB_NAME
DB_SSLMODE
```

For Neon, `DB_SSLMODE` must be:

```env
DB_SSLMODE=require
```

If the database connection fails during startup, the automatic migration step may fail before the server starts. Fix the Neon DB env vars and redeploy.

### Migration Fails

If a migration fails:

1. Open Render logs and identify the migration number.
2. Open Neon and inspect the database state.
3. Fix the SQL or data issue.
4. If the database is dirty, use `make migrate-force version=N` only after you know the correct version. Be careful: force changes the migration bookkeeping without running SQL.

### Email Startup Fails

If logs say email env is missing, either set:

```env
EMAIL_ENABLED=false
```

or provide:

```env
RESEND_API_KEY
EMAIL_FROM
```

### Redis Connection Warning

With:

```env
REDIS_ENABLED=false
```

there should be no Redis connection attempt. If you see Redis errors, confirm the env var is exactly:

```env
REDIS_ENABLED=false
```

Render env values are strings, so use lowercase `false`.

### Port Problem

Confirm:

```env
PORT=8080
```

The server binds to `:PORT`.

## 8. Deploy The Frontend On Cloudflare Pages

In Cloudflare Pages:

1. Click **Workers & Pages**.
2. Click **Create application**.
3. Choose **Pages**.
4. Connect GitHub.
5. Select `red-wave-app`.
6. Build settings for Angular:

```text
Framework preset: Angular
Build command: npm run build
Build output directory: dist/blueprint-audio/browser
```

If your build output path differs, check `angular.json` and the generated `dist` folder after a local build.

### Add `qa.waveyard.studio` To Cloudflare Pages

After the Pages project is created:

1. Open the Cloudflare Pages project.
2. Go to **Custom domains**.
3. Click **Set up a custom domain**.
4. Enter:

```text
qa.waveyard.studio
```

5. Follow Cloudflare's DNS instructions.

Because your domain is registered at Porkbun, you have two common choices:

- Recommended: move/manage DNS in Cloudflare by changing Porkbun nameservers to Cloudflare's nameservers.
- Alternative: keep Porkbun DNS and add the CNAME record Cloudflare Pages asks for.

The DNS record will usually be a CNAME similar to:

```text
qa -> your-cloudflare-pages-project.pages.dev
```

Wait for DNS and SSL certificate provisioning to finish before testing login/cookies.

## 9. Update Frontend Production API URL

In `red-wave-app`, update:

```text
src/environments/environment.prod.ts
```

Set:

```ts
export const environment = {
  production: true,
  apiUrl: 'https://api-qa.waveyard.studio',
  razorpayKeyId: 'rzp_live_your_key_id',
  googleClientId: 'your-google-client-id',
};
```

Commit and push. Cloudflare Pages will rebuild the frontend.

## 10. Update Render CORS After Frontend URL Is Known

After Cloudflare Pages deploys, copy the frontend URL and update Render:

```env
APP_BASE_URL=https://qa.waveyard.studio
ALLOWED_ORIGINS=https://qa.waveyard.studio,https://your-cloudflare-pages-url.pages.dev
```

Then in Render choose:

```text
Save and deploy
```

## 11. Smoke Test

Open the Render backend URL:

```text
https://blueprint-audio-api.onrender.com
```

If the root route does not exist, a 404 can still mean the service is running. Then test from the frontend:

1. Open Cloudflare Pages frontend.
2. Try signup/login.
3. Try listing specs/search.
4. Try an upload with a small file.
5. Check Render logs.
6. Check Neon tables/data.
7. Check R2 bucket objects.

## 12. Production Env Checklist

Render backend:

```env
PORT=8080
ENV=production
AUTO_MIGRATE=true
MIGRATIONS_PATH=db/migrations

DB_HOST=
DB_PORT=5432
DB_USER=
DB_PASSWORD=
DB_NAME=
DB_SSLMODE=require

REDIS_ENABLED=false

JWT_SECRET=
JWT_EXPIRATION=24h
JWT_REFRESH_EXPIRATION=720h
GOOGLE_CLIENT_ID=

USE_S3=true
S3_ENDPOINT=
S3_ACCESS_KEY=
S3_SECRET_KEY=
S3_BUCKET=
S3_USE_SSL=true
S3_REGION=auto
S3_PUBLIC_ENDPOINT=

RAZORPAY_KEY_ID=
RAZORPAY_KEY_SECRET=

APP_BASE_URL=
ALLOWED_ORIGINS=

EMAIL_ENABLED=false
RESEND_API_KEY=
EMAIL_FROM=
EMAIL_REPLY_TO=

RATE_LIMIT_ANONYMOUS=100
RATE_LIMIT_AUTHENTICATED=1000
```

Frontend production:

```ts
apiUrl: 'https://your-render-backend-url.onrender.com'
razorpayKeyId: 'rzp_live_or_test_key'
googleClientId: 'your-google-client-id'
```

## 13. Notes About Free Tier

Render free web services can sleep after inactivity. First request after sleep may be slow. This is acceptable for early testing.

Neon free Postgres can also scale down when idle. First DB request after idle can be slower.

When users increase, upgrade in this order:

1. Render backend paid instance, so it does not sleep.
2. Neon paid plan if DB limits become tight.
3. Add Redis later only if caching becomes necessary.

## 14. Official Docs Used

- Render Web Services: https://render.com/docs/web-services
- Render Environment Variables: https://render.com/docs/configure-environment-variables
- Render default `PORT` behavior: https://render.com/docs/environment-variables
- Neon connection details: https://neon.com/docs/get-started-with-neon/connect-neon
- Neon connection pooling: https://neon.com/docs/connect/connection-pooling
