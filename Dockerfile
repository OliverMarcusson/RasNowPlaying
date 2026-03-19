FROM --platform=$BUILDPLATFORM golang:1.26 AS build
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -o /out/sender ./cmd/sender

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /out/sender /app/sender
ENTRYPOINT ["/app/sender"]
