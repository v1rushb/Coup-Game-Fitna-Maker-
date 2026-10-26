# NEON//COUP relay (and, with a different CMD, any other neoncoup command).
#   docker build -t neoncoup .
#   docker run -d --name neoncoup-relay -p 8080:8080 neoncoup
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=docker
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/neoncoup ./cmd/neoncoup

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/neoncoup /usr/local/bin/neoncoup
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/neoncoup"]
CMD ["relay", "--addr", ":8080"]
