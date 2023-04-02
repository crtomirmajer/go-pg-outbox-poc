# Usage

Spin up:
```
make docker
make up
```

Scale producers:
```
make scale svc=producer n=1
```

Spin down:
```
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
