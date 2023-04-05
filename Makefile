svc=producer
n=1

binary:
	@go build -o pgoutbox cmd/service/main.go 

docker:
	@docker build --tag pgoutbox:0.0.1 . --progress=plain

up:
	@cd deploy && docker-compose up -d

scale:
	@cd deploy && docker-compose up -d --scale $(svc)=$(n)

down:
	@cd deploy && docker-compose down
