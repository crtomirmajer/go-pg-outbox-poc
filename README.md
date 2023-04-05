# About

This is a proof-of-concept of Outbox Pattern implementation using Go and Postgres, leveraging [Logical Decoding Messages](https://www.postgresql.org/docs/current/protocol-logicalrep-message-formats.html). 
Instead of using a typical Outbox table and listening to the changes on it, messages are written directly to WAL, using [pg_logical_emit_message](https://www.postgresql.org/docs/current/functions-admin.html#FUNCTIONS-REPLICATION). Inspired by [Wonders of Postgres Logical Decoding Messages](https://www.infoq.com/articles/wonders-of-postgres-logical-decoding-messages/) blog post.

## How to run the project?

Spin up:
```
make docker
make up
```

Scale producers:
```
make scale svc=producer n=2
```

Spin down:
```
make down
```

## How is latency measured?

Latency in the `consumer` is reporting end-to-end latency. That's *before* transactions start (in the `producer`), and right *after* deserialisation is finished in the `consumer`. Timeline view:

```
                                            latency
          ________________________________________________________________________________
         |                                                                                |
---------|------------------|----------------|---------------------|----------------------|------------------> time
    msg-created         tx-start        tx-finished       wal-message-consumed       message-deserialised

```

## Postgres utils

```
-- create a slot with `test_decoding` plugin
SELECT * FROM pg_create_logical_replication_slot('outbox_test', 'test_decoding');

-- read new WAL changes
SELECT * FROM pg_logical_slot_get_changes('outbox_test', NULL, NULL);

-- drop slot
SELECT pg_drop_replication_slot('outbox_test');

-- get info about replication slot state
SELECT * from pg_replication_slots;
```
