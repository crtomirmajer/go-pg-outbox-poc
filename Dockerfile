FROM golang:1.18 as builder

COPY . /build
WORKDIR /build
RUN ls
RUN go mod download
RUN CGO_ENABLED=0 go build -o pgoutbox ./cmd/service/main.go

FROM alpine:3.17.3

RUN mkdir /app
COPY --from=builder /build/pgoutbox /app

RUN apk add --no-cache curl ca-certificates && update-ca-certificates

WORKDIR /app
RUN ls && chmod +x pgoutbox

EXPOSE 4195

CMD ["./pgoutbox"]
