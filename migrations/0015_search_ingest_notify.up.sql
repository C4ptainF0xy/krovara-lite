CREATE OR REPLACE FUNCTION krovara_search_ingest_notify() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.host = 'conference.krovara.local'
       AND NEW.store = 'muc_log'
       AND NEW.type = 'xml' THEN
        PERFORM pg_notify('search_ingest', NEW.sort_id::text);
    END IF;
    RETURN NEW;
END;
$$;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables
               WHERE table_schema = 'public' AND table_name = 'prosodyarchive') THEN
        IF NOT EXISTS (SELECT 1 FROM pg_trigger
                       WHERE tgname = 'krovara_search_ingest') THEN
            EXECUTE 'CREATE TRIGGER krovara_search_ingest
                     AFTER INSERT ON prosodyarchive
                     FOR EACH ROW EXECUTE FUNCTION krovara_search_ingest_notify()';
        END IF;
    END IF;
END;
$$;
