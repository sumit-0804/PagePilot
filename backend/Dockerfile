# Builds a small static image for the API (used by Render).
FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /pagepilot ./cmd/server

# Distroless includes CA certificates for the HTTPS calls to Supabase, Groq and Gemini.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /pagepilot /pagepilot
EXPOSE 8080
ENTRYPOINT ["/pagepilot"]
