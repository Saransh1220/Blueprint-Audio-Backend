# Email Auth Curl Examples

Replace the example values before running these.

Base URL used below:

```bash
http://localhost:8080
```

## 1. Register

```bash
curl -X POST "http://localhost:8080/register" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "your-email@example.com",
    "password": "password123",
    "name": "Test User",
    "display_name": "Test User",
    "role": "artist"
  }'
```

## 2. Resend verification email

```bash
curl -X POST "http://localhost:8080/auth/resend-verification" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "your-email@example.com"
  }'
```

## 3. Verify email with code

```bash
curl -X POST "http://localhost:8080/auth/verify-email" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "your-email@example.com",
    "code": "123456"
  }'
```

## 4. Login

```bash
curl -X POST "http://localhost:8080/login" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "your-email@example.com",
    "password": "password123"
  }'
```

## 5. Forgot password

```bash
curl -X POST "http://localhost:8080/auth/forgot-password" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "your-email@example.com"
  }'
```

## 6. Reset password with code

```bash
curl -X POST "http://localhost:8080/auth/reset-password" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "your-email@example.com",
    "code": "123456",
    "new_password": "newpassword123"
  }'
```

## 7. Login with new password

```bash
curl -X POST "http://localhost:8080/login" \
  -H "Content-Type: application/json" \
  -d '{
    "email": "your-email@example.com",
    "password": "newpassword123"
  }'
```

## 8. Get current user after login

Replace `YOUR_JWT_TOKEN` with the token from `/login`.

```bash
curl -X GET "http://localhost:8080/me" \
  -H "Authorization: Bearer YOUR_JWT_TOKEN"
```

## Postman Tips

- Import any `curl` by using Postman Import and pasting the command.
- For verification and reset, use the 6-digit code from the email.
- `/auth/resend-verification` and `/auth/forgot-password` return generic success messages even when the backend intentionally hides whether an account exists.
