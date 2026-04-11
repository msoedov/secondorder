-- FTS5 full-text search index for wiki pages.
-- Indexes title, slug, and content for word-level search with BM25 ranking.
-- The content= and content_rowid= options make this an "external content" table
-- that mirrors wiki_pages without duplicating storage.

CREATE VIRTUAL TABLE IF NOT EXISTS wiki_pages_fts USING fts5(
    title,
    slug,
    content,
    content='wiki_pages',
    content_rowid='rowid'
);

-- Populate FTS index from existing wiki pages.
INSERT INTO wiki_pages_fts(rowid, title, slug, content)
    SELECT rowid, title, slug, content FROM wiki_pages;

-- Keep FTS index in sync: INSERT trigger.
CREATE TRIGGER IF NOT EXISTS wiki_pages_ai AFTER INSERT ON wiki_pages BEGIN
    INSERT INTO wiki_pages_fts(rowid, title, slug, content)
        VALUES (new.rowid, new.title, new.slug, new.content);
END;

-- Keep FTS index in sync: DELETE trigger.
CREATE TRIGGER IF NOT EXISTS wiki_pages_ad AFTER DELETE ON wiki_pages BEGIN
    INSERT INTO wiki_pages_fts(wiki_pages_fts, rowid, title, slug, content)
        VALUES ('delete', old.rowid, old.title, old.slug, old.content);
END;

-- Keep FTS index in sync: UPDATE trigger.
CREATE TRIGGER IF NOT EXISTS wiki_pages_au AFTER UPDATE ON wiki_pages BEGIN
    INSERT INTO wiki_pages_fts(wiki_pages_fts, rowid, title, slug, content)
        VALUES ('delete', old.rowid, old.title, old.slug, old.content);
    INSERT INTO wiki_pages_fts(rowid, title, slug, content)
        VALUES (new.rowid, new.title, new.slug, new.content);
END;
