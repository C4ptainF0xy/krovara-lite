DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables
               WHERE table_schema = 'public' AND table_name = 'prosodyarchive') THEN
        IF NOT EXISTS (SELECT 1 FROM pg_indexes
                       WHERE schemaname = 'public' AND indexname = 'idx_prosodyarchive_muc_sort') THEN
            EXECUTE 'CREATE INDEX idx_prosodyarchive_muc_sort
                     ON prosodyarchive (host, "user", store, sort_id)';
        END IF;
    END IF;
END;
$$;
