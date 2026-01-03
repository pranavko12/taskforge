FROM golang:1.24 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/worker ./cmd/worker

FROM gcr.io/distroless/static-debian12
WORKDIR /
COPY --from=build /out/worker /worker
ENTRYPOINT ["/worker"]
