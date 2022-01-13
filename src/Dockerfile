FROM golang:1.17.6-alpine as builder

WORKDIR /src
COPY .. .

RUN go get -d -v -t
RUN go build -v -o observer
RUN chmod +x observer

FROM alpine:latest

WORKDIR /src
COPY --from=builder /src/observer .

EXPOSE 8080
ENTRYPOINT ["/src/observer"]