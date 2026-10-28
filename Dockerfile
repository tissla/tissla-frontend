FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY main.go i18n.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /tissla-front .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /tissla-front /app/tissla-front
COPY templates ./templates
COPY static ./static
COPY content ./content
USER 65532:65532
ENV PORT=8333
EXPOSE 8333
ENTRYPOINT ["/app/tissla-front"]
