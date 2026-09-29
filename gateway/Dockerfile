FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /gateway .

FROM alpine:3.21
RUN adduser -D -H gateway && mkdir /data && chown gateway:gateway /data
USER gateway
WORKDIR /data
COPY --from=build /gateway /gateway
VOLUME ["/data"]
EXPOSE 2219
ENTRYPOINT ["/gateway"]
