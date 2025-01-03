FROM golang:1.23-alpine AS go-builder

ARG TARGETOS
ARG TARGETARCH

RUN mkdir -p /go/src/github.com/datasance/EdgeGuard
WORKDIR /go/src/github.com/datasance/EdgeGuard
COPY . /go/src/github.com/datasance/EdgeGuard
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o bin/EdgeGuard



# FROM alpine:3.20
FROM scratch
COPY LICENSE /licenses/LICENSE
COPY --from=go-builder /go/src/github.com/datasance/EdgeGuard/bin/EdgeGuard /bin/EdgeGuard

ENTRYPOINT [ "/bin/EdgeGuard" ]
