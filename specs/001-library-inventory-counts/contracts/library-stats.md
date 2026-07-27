# API Contract Reference: Library Inventory Endpoints

**Feature**: Library Inventory Counts Fix & Expansion
**Branch**: `001-library-inventory-counts`
**Date**: 2026-07-26

## Overview

The Library Settings page consumes three REST endpoints to assemble inventory metrics.

---

## 1. List Movies Endpoint

- **HTTP Method**: `GET`
- **Path**: `/api/v1/movies`
- **Query Parameters**:
  - `all=true`: Fetch all movie records without pagination tokening
- **Headers**:
  - `Authorization`: `Bearer <jwt_token>`
- **Response**: `200 OK`
  ```json
  [
    {
      "id": "3fa85f64-5717-4562-b3fc-2c963f66afa6",
      "library_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
      "title": "Inception",
      "file_path": "/media/movies/Inception.mkv",
      "duration": 8880.0,
      "transcode_status": "done"
    }
  ]
  ```

---

## 2. List All Episodes Endpoint

- **HTTP Method**: `GET`
- **Path**: `/api/v1/episodes`
- **Headers**:
  - `Authorization`: `Bearer <jwt_token>`
- **Response**: `200 OK`
  ```json
  [
    {
      "id": "e4a2c1f9-8d7b-4e12-a567-3b9e8f123456",
      "library_id": "c8b4d2e1-7f9a-4c35-8b1e-9a2f3d4e5f6a",
      "tv_show_id": "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
      "title": "Pilot",
      "season_number": 1,
      "episode_number": 1,
      "media_type": "episode"
    }
  ]
  ```

---

## 3. List TV Shows Endpoint

- **HTTP Method**: `GET`
- **Path**: `/api/v1/tv-shows`
- **Query Parameters**:
  - `library_id={uuid}`: Restrict results to a specific TV library
- **Headers**:
  - `Authorization`: `Bearer <jwt_token>`
- **Response**: `200 OK`
  ```json
  [
    {
      "id": "a1b2c3d4-e5f6-7a8b-9c0d-1e2f3a4b5c6d",
      "library_id": "c8b4d2e1-7f9a-4c35-8b1e-9a2f3d4e5f6a",
      "name": "Breaking Bad",
      "first_air_year": 2008
    }
  ]
  ```
