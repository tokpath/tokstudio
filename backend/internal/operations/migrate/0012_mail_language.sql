ALTER TABLE operations_preferences ADD COLUMN language TEXT NOT NULL DEFAULT 'zh' CHECK (language IN ('zh','en','ja','ru'));
