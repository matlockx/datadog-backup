FROM golang:1.14.0-alpine3.11 as go-builder

ENV PACKAGE github.com/matlockx/datadog-backup
ENV CGO_ENABLED 0

WORKDIR $GOPATH/src/$PACKAGE

# create directories for binary and install dependencies
RUN mkdir -p /out && apk --no-cache add git

COPY . ./
RUN go vet ./...
RUN go test --parallel=1 ./...
RUN go build -v -ldflags="-s -w" -o /out/datadog-backup ./cmd/datadog-backup


# build the final container image
FROM alpine:3.11

COPY --from=go-builder /out/datadog-backup /

ENTRYPOINT ["/datadog-backup"]