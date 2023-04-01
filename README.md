# Usage

```
make docker
make up
make down
```

# About latency

Latency in the consumer is reporting end-to-end latency before transactions start (producer) and after deserialised in consumer. Timeline:

```
                                            latency
          ________________________________________________________________________________
         |                                                                                |
---------|------------------|----------------|---------------------|----------------------|------------------> time
    msg-created         tx-start        tx-finished       wal-message-consumed       message-deserialised

```
