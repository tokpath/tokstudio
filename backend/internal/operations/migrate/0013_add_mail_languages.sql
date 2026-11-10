ALTER TABLE operations_preferences DROP CONSTRAINT operations_preferences_language_check;
ALTER TABLE operations_preferences ADD CONSTRAINT operations_preferences_language_check
    CHECK (language IN ('zh','en','ja','ru','hi','ms','th'));
