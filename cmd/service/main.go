package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/bluelabs-eu/pg-outbox/pkg/worker/consumer"
	"github.com/bluelabs-eu/pg-outbox/pkg/worker/producer"
)

var (
	host             = os.Getenv("DB_HOST")
	connectionString = "user=postgres password=test host=" + host + " port=5432 dbname=postgres"
)

type Worker interface {
	Run(context.Context) error
	Close(context.Context) error
}

func main() {

	mode := flag.String("mode", "produce", "(produce|consume)")
	flag.Parse()

	ctx, cancel := context.WithCancel(context.Background())
	runOnSignal(cancel, syscall.SIGTERM, syscall.SIGINT)

	if mode == nil {
		panic(fmt.Errorf("mode not set"))
	}

	var worker Worker
	var err error
	switch *mode {
	case "produce":
		worker, err = producer.New(ctx, connectionString)
	case "consume":
		// Use streaming-replication protocol with replication=1 (https://www.postgresql.org/docs/current/protocol-replication.html)
		// To initiate streaming replication, the frontend sends the replication parameter in the startup message.
		// A Boolean value of true (or on, yes, 1) tells the backend to go into physical replication walsender mode,
		// wherein a small set of replication commands, shown below, can be issued instead of SQL statements.
		worker, err = consumer.New(ctx, connectionString+" replication=database")
	default:
		err = fmt.Errorf("%v not implemented", mode)
	}
	if err != nil {
		panic(err)
	}

	defer worker.Close(ctx)
	err = worker.Run(ctx)
	if err != nil {
		panic(err)
	}
}

func runOnSignal(f func(), sig ...os.Signal) {
	stop := make(chan os.Signal, 5)
	signal.Notify(stop, sig...)
	go func() {
		<-stop
		f()
	}()
}
