FROM golang:1.26-alpine AS builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 go build -o /app ./cmd/app

FROM alpine:3.21

RUN apk add --no-cache tzdata ca-certificates

WORKDIR /app
COPY --from=builder /app .
COPY config/vendor_scenarios.yaml config/vendor_scenarios.yaml
COPY web/templates/ web/templates/
COPY web/static/ web/static/
COPY db/migrations/ db/migrations/
COPY entrypoint.sh .
RUN chmod +x entrypoint.sh

EXPOSE 8080
ENTRYPOINT ["./entrypoint.sh"]
