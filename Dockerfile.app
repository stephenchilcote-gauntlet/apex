FROM alpine:3.21

RUN apk add --no-cache tzdata

WORKDIR /app
COPY bin/app .
COPY config/vendor_scenarios.yaml config/vendor_scenarios.yaml
COPY web/templates/ web/templates/
COPY web/static/ web/static/
COPY db/migrations/ db/migrations/

EXPOSE 8080
CMD ["./app"]
