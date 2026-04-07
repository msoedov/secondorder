-- Migration 014: Update Gemini and Codex models to latest versions
UPDATE agents SET model = 'gemini-3.1-pro' WHERE runner = 'gemini' AND (model LIKE 'gemini-1.5%' OR model LIKE 'gemini-2.0%' OR model = 'gemini-3-flash-preview' OR model = 'gemini-3-pro' OR model = 'gemini-3-flash');
UPDATE agents SET model = 'gpt-5.4-thinking' WHERE runner = 'codex' AND (model LIKE 'gpt-4%' OR model = 'o4-mini');
UPDATE audit_runs SET model = 'gemini-3.1-pro' WHERE runner = 'gemini' AND (model LIKE 'gemini-1.5%' OR model LIKE 'gemini-2.0%' OR model = 'gemini-3-flash-preview' OR model = 'gemini-3-pro' OR model = 'gemini-3-flash');
UPDATE audit_runs SET model = 'gpt-5.4-thinking' WHERE runner = 'codex' AND (model LIKE 'gpt-4%' OR model = 'o4-mini');
