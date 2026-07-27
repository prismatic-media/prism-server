# Quickstart & Manual Verification Guide

**Feature**: Library Inventory Counts Fix & Expansion
**Branch**: `001-library-inventory-counts`
**Date**: 2026-07-26

## Prerequisites

1. Running Go backend server and compiled Angular frontend (`make dev`).
2. Administrator user credentials (`admin` / `asdf`).

---

## Verification Steps

### Step 1: Build & Launch Development Server
From repository root:
```bash
make dev
```
Verify that frontend compiles cleanly and backend starts on port 8080.

### Step 2: Access Library Settings
1. Open browser to `http://localhost:8080`.
2. Log in using `admin` / `asdf`.
3. Navigate to **Library Settings** (`/admin/library`).

### Step 3: Verify Movie Inventory Count Fix
1. Inspect the "Total Inventory" bento card.
2. Confirm the **Movies** breakdown item shows the actual non-zero count of indexed movies (matching the number of movie files indexed).
3. If no movies are indexed, confirm it shows `0` without error.

### Step 4: Verify Episodes Inventory Item
1. Locate the new **Episodes** row in the Total Inventory card.
2. Verify it displays an icon (`video_library` symbol in amber) and a count matching the total number of TV episodes across all series.

### Step 5: Dynamic Update Test
1. Click **Scan All Libraries** or scan a specific TV/Movie library.
2. Ensure that upon completion or page reload, updated movie and episode totals immediately render without full client application reload.
