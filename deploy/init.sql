CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  first_name TEXT,
  details JSONB,
  birth_date DATE
);

-- Note how publication doesn't include any TABLES - we'll only receive logical decoding messages, without table change-data.
-- NOTE: Is it event necessary to create it?
CREATE PUBLICATION outbox_publication;

-- Create a replication slot for "outbox messages", used in the consumer
-- Use `pgoutput` - the standard logical decoding plugin (https://github.com/postgres/postgres/tree/master/src/backend/replication/pgoutput)
SELECT * FROM pg_create_logical_replication_slot('outbox_slot', 'pgoutput');
