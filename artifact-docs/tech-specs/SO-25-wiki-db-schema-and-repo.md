# SO-25 Wiki DB Schema and Repository Layer

## Overview

This change introduces persistent storage for wiki pages in SQLite and adds repository methods in the DB layer for CRUD operations.

## Database Schema

Migration: `internal/db/migrations/023_wiki_pages.sql`

New table: `wiki_pages`

- `id` (TEXT PRIMARY KEY)
- `slug` (TEXT NOT NULL UNIQUE)
- `title` (TEXT NOT NULL)
- `content` (TEXT NOT NULL DEFAULT '')
- `created_by_agent_id` (TEXT, nullable, FK -> `agents.id`)
- `updated_by_agent_id` (TEXT, nullable, FK -> `agents.id`)
- `created_at` (DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)
- `updated_at` (DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP)

Indexes:

- `idx_wiki_pages_slug` on `slug`
- `idx_wiki_pages_updated_at` on `updated_at`

## Model

Added `models.WikiPage` in `internal/models/models.go`:

- `ID`, `Slug`, `Title`, `Content`
- `CreatedByAgentID`, `UpdatedByAgentID`
- `CreatedAt`, `UpdatedAt`

## Repository API

Added DB methods in `internal/db/queries.go`:

- `CreateWikiPage(p *models.WikiPage) error`
- `GetWikiPageBySlug(slug string) (*models.WikiPage, error)`
- `ListWikiPages() ([]models.WikiPage, error)`
- `UpdateWikiPage(p *models.WikiPage) error`
- `DeleteWikiPage(id string) error`

These methods follow existing repository conventions in the codebase:

- UUID generation on create when ID is missing
- UTC timestamps from Go for create/update operations
- SQL and scan patterns aligned with existing `internal/db/queries.go` methods
