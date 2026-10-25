"""Telegram -> start/stop/status for the dev EC2 instance, plus an auto-stop safety net.

Triggers:
  * Lambda Function URL (Telegram webhook)  -> /start /stop /status /help
  * EventBridge schedule (every 30 min)     -> stops the instance if up longer than MAX_UPTIME_HOURS

Env vars: INSTANCE_ID, BOT_TOKEN, WEBHOOK_SECRET, ALLOWED_CHAT_IDS (comma list),
          MAX_UPTIME_HOURS (default 4), DEV_API_URL, DEV_UI_URL
"""
import json
import os
import urllib.parse
import urllib.request
from datetime import datetime, timezone

import boto3

ec2 = boto3.client("ec2")
INSTANCE_ID = os.environ["INSTANCE_ID"]
BOT_TOKEN = os.environ["BOT_TOKEN"]
WEBHOOK_SECRET = os.environ["WEBHOOK_SECRET"]
ALLOWED = {int(x) for x in os.environ["ALLOWED_CHAT_IDS"].split(",") if x.strip()}
MAX_HOURS = float(os.environ.get("MAX_UPTIME_HOURS", "4"))
API_URL = os.environ.get("DEV_API_URL", "")
UI_URL = os.environ.get("DEV_UI_URL", "")


def instance():
    r = ec2.describe_instances(InstanceIds=[INSTANCE_ID])
    return r["Reservations"][0]["Instances"][0]


def uptime_hours(inst):
    return (datetime.now(timezone.utc) - inst["LaunchTime"]).total_seconds() / 3600


def say(chat_id, text):
    data = urllib.parse.urlencode({"chat_id": chat_id, "text": text}).encode()
    urllib.request.urlopen(
        f"https://api.telegram.org/bot{BOT_TOKEN}/sendMessage", data=data, timeout=10
    )


def handle_command(chat_id, cmd):
    inst = instance()
    state = inst["State"]["Name"]
    if cmd == "/start":
        if state in ("running", "pending"):
            return f"Already {state}. API: {API_URL}"
        if state != "stopped":
            return f"Instance is {state}, try again in a minute."
        ec2.start_instances(InstanceIds=[INSTANCE_ID])
        return (f"Starting... I'll message you when the API is healthy (~1-2 min). "
                f"Auto-stops after {MAX_HOURS:g}h.")
    if cmd == "/stop":
        if state in ("stopped", "stopping"):
            return f"Already {state}."
        ec2.stop_instances(InstanceIds=[INSTANCE_ID])
        return "Stopping. No compute cost once it is stopped."
    if cmd == "/status":
        extra = f", up {uptime_hours(inst):.1f}h" if state == "running" else ""
        return f"State: {state}{extra}\nUI: {UI_URL}\nAPI: {API_URL}"
    return "Commands: /start /stop /status"


def lambda_handler(event, context):
    # Scheduled auto-stop
    if event.get("source") == "aws.events":
        inst = instance()
        if inst["State"]["Name"] == "running" and uptime_hours(inst) > MAX_HOURS:
            ec2.stop_instances(InstanceIds=[INSTANCE_ID])
            for cid in ALLOWED:
                say(cid, f"Auto-stopped dev server after {MAX_HOURS:g}h. Send /start to bring it back.")
        return {"ok": True}

    # Telegram webhook
    headers = {k.lower(): v for k, v in (event.get("headers") or {}).items()}
    if headers.get("x-telegram-bot-api-secret-token") != WEBHOOK_SECRET:
        return {"statusCode": 403, "body": "forbidden"}
    msg = (json.loads(event.get("body") or "{}")).get("message") or {}
    chat_id = (msg.get("chat") or {}).get("id")
    text = (msg.get("text") or "").strip().split()
    if chat_id not in ALLOWED or not text:
        return {"statusCode": 200, "body": "ignored"}
    cmd = text[0].split("@")[0].lower()
    try:
        say(chat_id, handle_command(chat_id, cmd))
    except Exception as e:  # report the failure instead of letting Telegram retry forever
        say(chat_id, f"Error: {e}")
    return {"statusCode": 200, "body": "ok"}
