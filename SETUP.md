# StarkLabs Setup Guide

## What is already added
- `go.mod`
- `main.go`
- `docs/index.html`
- `docs/signals.json`
- `.github/workflows/scanner.yml`

## What this gives you
- A lightweight market-scanner MVP in Go
- Static dashboard for GitHub Pages
- Scheduled signal refresh every 10 minutes on weekdays

## Current mode
The project currently runs in `mock` mode by default so it works immediately without paid APIs.

## To switch to live mode later
Add repository secrets:
- `DATA_PROVIDER=alpaca`
- `ALPACA_API_KEY`
- `ALPACA_API_SECRET`
- `SMTP_HOST`
- `SMTP_PORT`
- `SMTP_USER`
- `SMTP_PASS`
- `ALERT_TO`

## To publish the dashboard
In GitHub repository settings:
1. Open **Pages**
2. Set source to **Deploy from a branch**
3. Choose branch `master`
4. Choose folder `/docs`

## To run the workflow immediately
In GitHub:
1. Open **Actions**
2. Select **Market Scanner**
3. Click **Run workflow**

## Important note
This is a scanner MVP and not a guaranteed-trading system. Use backtesting and risk limits before acting on any signal.
