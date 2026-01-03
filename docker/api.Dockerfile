FROM golang:1.24 AS build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/api ./cmd/api

FROM gcr.io/distroless/static-debian12
WORKDIR /
COPY --from=build /out/api /api
EXPOSE 8080
ENTRYPOINT ["/api"]
